package dto

import (
	"math"

	"github.com/gin-gonic/gin"
)

// APIResponse matches the user-service response envelope:
// {"status":"success|error","statusCode":200,"message":"...","data":{...}}
type APIResponse struct {
	Status     string      `json:"status"`
	StatusCode int         `json:"statusCode"`
	Message    string      `json:"message"`
	Data       interface{} `json:"data,omitempty"`
	Error      string      `json:"error,omitempty"`
}

// PaginatedData wraps paginated results.
type PaginatedData struct {
	Items      interface{} `json:"items"`
	Pagination Pagination  `json:"pagination"`
}

// Pagination metadata matching restaurant-service pattern.
type Pagination struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// Success sends a successful JSON response.
func Success(c *gin.Context, statusCode int, message string, data interface{}) {
	c.JSON(statusCode, APIResponse{
		Status:     "success",
		StatusCode: statusCode,
		Message:    message,
		Data:       data,
	})
}

// ErrorResponse sends an error JSON response.
func ErrorResponse(c *gin.Context, statusCode int, message string, errDetail string) {
	c.JSON(statusCode, APIResponse{
		Status:     "error",
		StatusCode: statusCode,
		Message:    message,
		Error:      errDetail,
	})
}

// ValidationError sends a 400 validation error.
func ValidationError(c *gin.Context, message string) {
	ErrorResponse(c, 400, message, "Validation failed")
}

// Unauthorized sends a 401 response.
func Unauthorized(c *gin.Context, message string) {
	ErrorResponse(c, 401, message, "Unauthorized")
}

// Forbidden sends a 403 response.
func Forbidden(c *gin.Context, message string) {
	ErrorResponse(c, 403, message, "Forbidden")
}

// NotFound sends a 404 response.
func NotFound(c *gin.Context, message string) {
	ErrorResponse(c, 404, message, "Not found")
}

// Conflict sends a 409 response.
func Conflict(c *gin.Context, message string) {
	ErrorResponse(c, 409, message, "Conflict")
}

// InternalError sends a 500 response.
func InternalError(c *gin.Context, message string) {
	ErrorResponse(c, 500, message, "Internal server error")
}

// Paginated sends a paginated success response.
func Paginated(c *gin.Context, items interface{}, page, limit int, total int64) {
	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	Success(c, 200, "fetched", PaginatedData{
		Items: items,
		Pagination: Pagination{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}
