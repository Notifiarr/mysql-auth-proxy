package webserver

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

/* High performance access logging. */

const accessLogInitialGrow = 512

// responseWriter records status and body size for access logging.
type responseWriter struct {
	http.ResponseWriter

	start  time.Time
	status int
	size   int64
}

func (w *responseWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}

	w.ResponseWriter.WriteHeader(code)
}

func (w *responseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}

	n, err := w.ResponseWriter.Write(p)
	w.size += int64(n)

	return n, err //nolint:wrapcheck // delegate to underlying ResponseWriter
}

func (w *responseWriter) statusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}

	return w.status
}

// Get returns the first value for a response header field. key must already be in
// canonical form (http.CanonicalHeaderKey). Unlike Header.Get it does not allocate
// or re-canonicalize key on each call.
func (w *responseWriter) Get(key string) string {
	if v := w.Header()[key]; len(v) > 0 {
		return v[0]
	}

	return ""
}

//nolint:gochecknoglobals // one pool per process for hot-path access log strings.Builder reuse
var alBuilder = sync.Pool{New: func() any { return &strings.Builder{} }}

// accessLogWrap writes one Apache-style line per request to dst (same field order as the former alFmt).
func accessLogWrap(next http.Handler, dst io.Writer) http.Handler {
	return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		writer := &responseWriter{ResponseWriter: resp, start: time.Now()}
		next.ServeHTTP(writer, req)
		writer.writeAccessLogLine(req, dst)
	})
}

func (w *responseWriter) writeAccessLogLine(req *http.Request, dst io.Writer) {
	//nolint:forcetypeassert
	builder := alBuilder.Get().(*strings.Builder) // Get a string buffer:
	builder.Reset()                               //  - reset it.
	builder.Grow(accessLogInitialGrow)            //  - grow it.
	w.writeAccessLogLinePrefix(builder, req)      //  - fill prefix.
	w.writeAccessLogLineTail(builder, req)        //  - fill suffix.
	_, _ = io.WriteString(dst, builder.String())  //  - write it.
	alBuilder.Put(builder)                        //  - put it back.
}

func (w *responseWriter) writeAccessLogLinePrefix(
	builder *strings.Builder,
	req *http.Request,
) {
	// %V
	builder.WriteString(req.Host)
	builder.WriteByte(' ')
	// %{X-Forwarded-For}i — computed like former fixForwardedFor (not raw header).
	builder.WriteString(ClientIPForLog(req))
	builder.WriteByte(' ')
	// "%{X-Username}o"
	builder.WriteByte('"')
	builder.WriteString(w.Get("X-Username"))
	builder.WriteString("\" ")
	// %{X-UserID}o
	builder.WriteString(w.Get("X-Userid"))
	builder.WriteByte(' ')
	// %t — [02/Jan/2006:15:04:05 -0700]
	builder.WriteByte('[')
	builder.WriteString(w.start.Format("02/Jan/2006:15:04:05 -0700"))
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
	builder.WriteString(strconv.Itoa(w.statusCode()))
	builder.WriteByte(' ')
	// %b — response body size (0 when none).
	builder.WriteString(strconv.FormatInt(w.size, 10))

	builder.WriteByte(' ')
	// "%{Referer}i" "%{User-agent}i" query:...
	builder.WriteByte('"')
	builder.WriteString(RefererPathForLog(req.Header))
	builder.WriteString("\" \"")
	builder.WriteString(req.UserAgent())
	builder.WriteByte('"')
}

func (w *responseWriter) writeAccessLogLineTail(
	builder *strings.Builder,
	req *http.Request,
) {
	builder.WriteString(" req:")
	// %{ms}T — elapsed milliseconds (same as apache-logformat request duration).
	builder.WriteString(strconv.FormatInt(time.Since(w.start).Milliseconds(), 10))
	builder.WriteString("ms age:")
	builder.WriteString(w.Get("Age"))
	builder.WriteString(" env:")
	builder.WriteString(w.Get("X-Environment"))
	builder.WriteString(" key:")

	masked, keyLenStr := w.maskedAPIKeyFromResponse()

	builder.WriteString(masked)
	builder.WriteByte('(')
	builder.WriteString(keyLenStr)
	builder.WriteString(") \"srv:")
	builder.WriteString(req.Header.Get("X-Server"))
	builder.WriteString("\"\n")
}

// maskedAPIKeyFromResponse returns maskAPIKey(w X-Api-Key) for the access log, or ("", "")
// when the handler did not set that response header.
func (w *responseWriter) maskedAPIKeyFromResponse() (string, string) {
	key := w.Get("X-Api-Key")
	if key == "" {
		return "", ""
	}

	return maskAPIKey(key)
}
