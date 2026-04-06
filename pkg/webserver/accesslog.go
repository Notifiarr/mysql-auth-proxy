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
	if c.status == 0 {
		c.status = code
	}

	c.ResponseWriter.WriteHeader(code)
}

func (c *captureWriter) Write(p []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}

	n, err := c.ResponseWriter.Write(p)
	c.size += int64(n)

	return n, err //nolint:wrapcheck // delegate to underlying ResponseWriter
}

func (c *captureWriter) statusCode() int {
	if c.status == 0 {
		return http.StatusOK
	}

	return c.status
}

// Get returns the first value for a response header field. key must already be in
// canonical form (http.CanonicalHeaderKey). Unlike Header.Get it does not allocate
// or re-canonicalize key on each call.
func (c *captureWriter) Get(key string) string {
	if v := c.Header()[key]; len(v) > 0 {
		return v[0]
	}

	return ""
}

//nolint:gochecknoglobals // one pool per process for hot-path access log strings.Builder reuse
var alBuilder = sync.Pool{New: func() any { return &strings.Builder{} }}

// accessLogWrap writes one Apache-style line per request to dst (same field order as the former alFmt).
func accessLogWrap(next http.Handler, dst io.Writer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		capture := &captureWriter{ResponseWriter: w, start: time.Now()}
		next.ServeHTTP(capture, req)
		capture.writeAccessLogLine(req, dst)
	})
}

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
	// %V
	builder.WriteString(req.Host)
	builder.WriteByte(' ')
	// %{X-Forwarded-For}i — computed like former fixForwardedFor (not raw header).
	builder.WriteString(ClientIPForLog(req))
	builder.WriteByte(' ')
	// "%{X-Username}o"
	builder.WriteByte('"')
	builder.WriteString(c.Get("X-Username"))
	builder.WriteString("\" ")
	// %{X-UserID}o
	builder.WriteString(c.Get("X-Userid"))
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
	builder.WriteString(strconv.Itoa(c.statusCode()))
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
	builder.WriteString(" req:")
	// %{ms}T — elapsed milliseconds (same as apache-logformat request duration).
	builder.WriteString(strconv.FormatInt(time.Since(c.start).Milliseconds(), 10))
	builder.WriteString("ms age:")
	builder.WriteString(c.Get("Age"))
	builder.WriteString(" env:")
	builder.WriteString(c.Get("X-Environment"))
	builder.WriteString(" key:")

	masked, keyLenStr := c.maskedAPIKeyFromResponse()
	builder.WriteString(masked)
	builder.WriteByte('(')
	builder.WriteString(keyLenStr)
	builder.WriteString(") \"srv:")
	builder.WriteString(req.Header.Get("X-Server"))
	builder.WriteString("\"\n")
}

// maskedAPIKeyFromResponse returns maskAPIKey(w X-Api-Key) for the access log, or ("", "")
// when the handler did not set that response header.
func (c *captureWriter) maskedAPIKeyFromResponse() (string, string) {
	key := c.Get("X-Api-Key")
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
	pathPart, _, _ := strings.Cut(header.Get("X-Original-Uri"), "?")
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
	forwarded := req.Header.Get("X-Forwarded-For")
	if forwarded == "" {
		host, _, err := net.SplitHostPort(req.RemoteAddr)
		if err != nil {
			return strings.Trim(req.RemoteAddr, "[]")
		}

		return host
	}

	return strings.TrimSpace(strings.Split(forwarded, ",")[0])
}
