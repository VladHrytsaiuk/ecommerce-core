package apiresponse

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/VladHrytsaiuk/ecommerce-core/internal/platform/logger"
)

// Code is a stable, machine-readable public error code. Its values are part
// of the v1 HTTP contract and are intentionally independent of Go error text.
type Code string

const (
	CodeInvalidPayload   Code = "INVALID_PAYLOAD"
	CodeValidationFailed Code = "VALIDATION_FAILED"
	CodeUnauthenticated  Code = "UNAUTHENTICATED"
	CodeForbidden        Code = "FORBIDDEN"
	CodeNotFound         Code = "RESOURCE_NOT_FOUND"
	CodeConflict         Code = "CONFLICT"
	CodeRateLimited      Code = "RATE_LIMITED"
	CodePayloadTooLarge  Code = "PAYLOAD_TOO_LARGE"
	CodeSearchUnavailable Code = "SEARCH_UNAVAILABLE"
	CodeInternal         Code = "INTERNAL_ERROR"
)

// InvalidParam describes one safe, client-actionable validation failure.
type InvalidParam struct {
	Field string `json:"field"`
	Code  string `json:"code"`
}

// ProblemDetails follows RFC 9457 and adds stable code, invalid parameter, and
// request correlation extensions for API clients.
type ProblemDetails struct {
	Type      string         `json:"type"`
	Title     string         `json:"title"`
	Status    int            `json:"status"`
	Detail    string         `json:"detail,omitempty"`
	Instance  string         `json:"instance,omitempty"`
	Code      Code           `json:"code"`
	Errors    []InvalidParam `json:"errors,omitempty"`
	RequestID string         `json:"request_id"`
}

// PublicError decorates an original error with safe, stable HTTP semantics.
// Cause is logged but never serialized into ProblemDetails.
type PublicError struct {
	Cause   error
	Status  int
	Code    Code
	Title   string
	Detail  string
	Invalid []InvalidParam
}

func (e *PublicError) Error() string {
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return string(e.Code)
}

func (e *PublicError) Unwrap() error { return e.Cause }

func InvalidPayload(cause error, fields ...InvalidParam) *PublicError {
	return &PublicError{Cause: cause, Status: http.StatusBadRequest, Code: CodeInvalidPayload, Title: "Invalid request payload", Detail: "The request body or parameters are invalid.", Invalid: fields}
}

func ValidationFailed(cause error, fields ...InvalidParam) *PublicError {
	return &PublicError{Cause: cause, Status: http.StatusUnprocessableEntity, Code: CodeValidationFailed, Title: "Validation failed", Detail: "One or more fields are invalid.", Invalid: fields}
}

func NotFound(cause error, detail string) *PublicError {
	return &PublicError{Cause: cause, Status: http.StatusNotFound, Code: CodeNotFound, Title: "Resource not found", Detail: detail}
}

func Unauthenticated(cause error) *PublicError {
	return &PublicError{Cause: cause, Status: http.StatusUnauthorized, Code: CodeUnauthenticated, Title: "Authentication required", Detail: "Valid authentication credentials are required."}
}

func Forbidden(cause error) *PublicError {
	return &PublicError{Cause: cause, Status: http.StatusForbidden, Code: CodeForbidden, Title: "Permission denied", Detail: "You do not have permission to perform this action."}
}

func Unavailable(cause error) *PublicError {
	return &PublicError{Cause: cause, Status: http.StatusServiceUnavailable, Code: CodeInternal, Title: "Service unavailable", Detail: "The authorization service is temporarily unavailable."}
}

func SearchUnavailable(cause error) *PublicError {
	return &PublicError{Cause: cause, Status: http.StatusServiceUnavailable, Code: CodeSearchUnavailable, Title: "Search unavailable", Detail: "Product search is temporarily unavailable."}
}

func RateLimited(cause error) *PublicError {
	return &PublicError{Cause: cause, Status: http.StatusTooManyRequests, Code: CodeRateLimited, Title: "Rate limit exceeded", Detail: "Too many requests. Please try again later."}
}

func PayloadTooLarge(cause error) *PublicError {
	return &PublicError{Cause: cause, Status: http.StatusRequestEntityTooLarge, Code: CodePayloadTooLarge, Title: "Payload too large", Detail: "The request body exceeds the supported size."}
}

// Classifier maps a module-owned error to a public transport error without
// coupling this generic package to module domains.
type Classifier func(error) (*PublicError, bool)

type ErrorRenderer struct {
	log         logger.Logger
	classifiers []Classifier
}

func NewErrorRenderer(log logger.Logger, classifiers ...Classifier) *ErrorRenderer {
	return &ErrorRenderer{log: log, classifiers: classifiers}
}

func (r *ErrorRenderer) WithClassifier(classifier Classifier) *ErrorRenderer {
	if classifier != nil {
		// Classifiers are registered only while Bootstrap/route construction is
		// single-threaded, before the server starts accepting requests.
		r.classifiers = append(r.classifiers, classifier)
	}
	return r
}

// Middleware renders errors placed in gin.Context by Abort. It is intentionally
// attached only to v1 routes while legacy endpoints retain their old wire form.
func (r *ErrorRenderer) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if c.Writer.Written() || len(c.Errors) == 0 {
			return
		}
		r.Render(c, c.Errors.Last().Err)
	}
}

func (r *ErrorRenderer) Abort(c *gin.Context, err error) {
	_ = c.Error(err)
	c.Abort()
}

func (r *ErrorRenderer) Render(c *gin.Context, err error) {
	if c.Writer.Written() {
		return
	}
	public := r.classify(err)
	requestID := requestID(c)
	if r.log != nil {
		logWithContext(r.log, c).Errorw("HTTP v1 request failed", "error", err, "code", public.Code, "status", public.Status, "request_id", requestID)
	}
	problem := ProblemDetails{
		Type:      "urn:ecommerce-core:problem:" + strings.ToLower(strings.ReplaceAll(string(public.Code), "_", "-")),
		Title:     public.Title,
		Status:    public.Status,
		Detail:    public.Detail,
		Instance:  problemInstance(requestID),
		Code:      public.Code,
		Errors:    public.Invalid,
		RequestID: requestID,
	}
	c.Header("Content-Type", "application/problem+json")
	c.JSON(public.Status, problem)
}

func (r *ErrorRenderer) classify(err error) *PublicError {
	var public *PublicError
	if errors.As(err, &public) {
		return public
	}
	var maxBytes *http.MaxBytesError
	if errors.As(err, &maxBytes) {
		return PayloadTooLarge(err)
	}
	for _, classifier := range r.classifiers {
		if classified, ok := classifier(err); ok && classified != nil {
			return classified
		}
	}
	return &PublicError{Cause: err, Status: http.StatusInternalServerError, Code: CodeInternal, Title: "Internal server error", Detail: "The server could not process the request."}
}

func problemInstance(requestID string) string {
	if requestID == "" {
		return ""
	}
	return fmt.Sprintf("urn:ecommerce-core:request:%s", requestID)
}

func logWithContext(log logger.Logger, c *gin.Context) logger.Logger {
	if contextual, ok := log.(logger.ContextLogger); ok {
		return contextual.WithContext(c.Request.Context())
	}
	return log
}
