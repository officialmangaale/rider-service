package middleware

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTrackingChecksAccessBeforeReturningLocations(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		authenticated, allowed bool
		want                   int
	}{
		{"anonymous", false, false, 401}, {"other rider", true, false, 403}, {"order participant", true, true, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if tc.authenticated {
				mock.ExpectQuery("SELECT EXISTS").WithArgs("100", "rider").WillReturnRows(sqlmock.NewRows([]string{"allowed"}).AddRow(tc.allowed))
			}
			router := gin.New()
			router.GET("/tracking/:orderId", AuthMiddleware("secret"), TrackingAccess(db), func(c *gin.Context) { c.JSON(200, gin.H{"private_location": "only for participants"}) })
			req := httptest.NewRequest(http.MethodGet, "/tracking/100", nil)
			if tc.authenticated {
				token, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "rider", "exp": time.Now().Add(time.Hour).Unix()}).SignedString([]byte("secret"))
				req.Header.Set("Authorization", "Bearer "+token)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status %d body %s", w.Code, w.Body.String())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
