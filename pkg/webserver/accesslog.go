package webserver

import (
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

/* High performance access logging. */

const accessLogInitialGrow = 512

// captureWriter records status and body size for access logging.
type captureWriter struct {
	http.ResponseWriter

	start  time.Time
	status int
	size   int64
}

func (c *captureWriter) WriteHeader(code int) {
	if c.status == 0 { // cannot set it twice.
		c.status = code
	}

	c.ResponseWriter.WriteHeader(code)
}

func (c *captureWriter) Write(p []byte) (int, error) {
	if c.status == 0 { // if you write without setting the status, it's a 200.
		c.status = http.StatusOK
	}

	n, err := c.ResponseWriter.Write(p)
	c.size += int64(n)

	return n, err //nolint:wrapcheck // delegate to underlying ResponseWriter
}

func (c *captureWriter) statusCode() string {
	if c.status == 0 {
		return "200"
	}

	return strconv.Itoa(c.status)
}

// accessLogWrap writes one Apache-style line per request to dst (same field order as the former alFmt)
// and records HTTP request/response Prometheus counters when metrics is non-nil.
func (s *server) accessLogWrap(next http.Handler, dst io.Writer) http.Handler {
	return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		capture := &captureWriter{ResponseWriter: resp, start: time.Now()}
		next.ServeHTTP(capture, req)
		capture.writeAccessLogLine(req, dst)
		// Update Prometheus metrics for the request.
		s.metrics.CountRequest(req, capture.statusCode())
	})
}

//nolint:gochecknoglobals // one pool per process for hot-path access log strings.Builder reuse
var alBuilder = sync.Pool{New: func() any { return &strings.Builder{} }}

func (c *captureWriter) writeAccessLogLine(req *http.Request, dst io.Writer) {
	//nolint:forcetypeassert
	builder := alBuilder.Get().(*strings.Builder) // Get a string buffer:
	builder.Reset()                               //  - reset it.
	builder.Grow(accessLogInitialGrow)            //  - grow it.
	c.writeAccessLogLinePrefix(builder, req)      //  - fill prefix.
	c.writeAccessLogLineTail(builder, req)        //  - fill suffix.
	_, _ = io.WriteString(dst, builder.String())  //  - write it.
	alBuilder.Put(builder)                        //  - put it back.
}

func (c *captureWriter) writeAccessLogLinePrefix(builder *strings.Builder, req *http.Request) {
	respHeader := c.Header()
	// %V
	builder.WriteString(req.Host)
	builder.WriteByte(' ')
	// %{X-Forwarded-For}i — computed like former fixForwardedFor (not raw header).
	builder.WriteString(ClientIPForLog(req))
	builder.WriteByte(' ')
	// "%{X-Username}o"
	builder.WriteByte('"')
	builder.WriteString(getHeader(respHeader, HeaderXUsername))
	builder.WriteString("\" ")
	// %{X-UserID}o
	builder.WriteString(getHeader(respHeader, HeaderXUserid))
	builder.WriteByte(' ')
	// %t — [02/Jan/2006:15:04:05 -0700]
	builder.WriteByte('[')
	builder.WriteString(c.start.Format("02/Jan/2006:15:04:05 -0700"))
	builder.WriteString("] ")
	// "%r"
	builder.WriteByte('"')
	builder.WriteString(req.Method)
	builder.WriteByte(' ')

	if req.RequestURI != "" {
		builder.WriteString(req.RequestURI)
	} else {
		builder.WriteString(req.URL.RequestURI())
	}

	builder.WriteString(" HTTP/1.1\" ")
	// %>s
	builder.WriteString(c.statusCode())
	builder.WriteByte(' ')
	// %b — response body size (0 when none).
	builder.WriteString(strconv.FormatInt(c.size, 10))

	builder.WriteByte(' ')
	// "%{Referer}i" "%{User-agent}i" query:...
	builder.WriteByte('"')
	builder.WriteString(RefererPathForLog(req.Header))
	builder.WriteString("\" \"")
	builder.WriteString(req.UserAgent())
	builder.WriteByte('"')
}

func (c *captureWriter) writeAccessLogLineTail(builder *strings.Builder, req *http.Request) {
	respHeader := c.Header()
	builder.WriteString(" req:")
	// %{ms}T — elapsed milliseconds (same as apache-logformat request duration).
	builder.WriteString(strconv.FormatInt(time.Since(c.start).Milliseconds(), 10))
	builder.WriteString("ms age:")
	builder.WriteString(getHeader(respHeader, HeaderAge))
	builder.WriteString(" env:")
	builder.WriteString(getHeader(respHeader, HeaderEnvironment))
	builder.WriteString(" key:")

	masked, keyLenStr := maskedAPIKeyForLog(req, respHeader)
	builder.WriteString(masked)
	builder.WriteByte('(')
	builder.WriteString(keyLenStr)
	builder.WriteString(") \"srv:")
	builder.WriteString(getHeader(req.Header, HeaderXServer))
	builder.WriteString("\"\n")
}

// maskedAPIKeyForLog returns maskAPIKey(X-Api-Key) from the response, else from the parsed request
// context, or ("", "") if neither is set.
func maskedAPIKeyForLog(req *http.Request, resp http.Header) (string, string) {
	key := getHeader(resp, HeaderXAPIKey)
	if key == "" {
		key = apiKeyFromRequest(req)
	}

	if key == "" {
		return "", ""
	}

	return maskAPIKey(key)
}

// RefererPathForLog returns the path part of X-Original-Uri (no query string) truncated before the
// API key segment (keyPosition), using the same strings.Split(path, "/") rules as GetAPIKeyFromURIPath.
// If the path has fewer than keyPosition+1 segments, it returns the full path (still without query).
// When X-Original-Uri is missing, empty, or only a query string, it returns "".
func RefererPathForLog(header http.Header) string {
	pathPart, _, _ := strings.Cut(getHeader(header, HeaderXOriginalURI), "?")
	if pathPart == "" {
		return ""
	}

	var pos, segIdx int

	for seg := range strings.SplitSeq(pathPart, "/") {
		if segIdx == keyPosition {
			return strings.TrimSuffix(pathPart[:pos], "/")
		}

		pos += len(seg)
		if pos < len(pathPart) && pathPart[pos] == '/' {
			pos++
		}

		segIdx++
	}

	return pathPart
}

// ClientIPForLog returns the client IP for access logs (same rules as the former fixForwardedFor middleware).
func ClientIPForLog(req *http.Request) string {
	forwarded := getHeader(req.Header, HeaderXForwardedFor)
	if forwarded == "" {
		host, _, err := net.SplitHostPort(req.RemoteAddr)
		if err != nil {
			return strings.Trim(req.RemoteAddr, "[]")
		}

		return host
	}

	return strings.TrimSpace(strings.Split(forwarded, ",")[0])
}
