package router

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/handler"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/middleware"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/service"
)

// Setup creates all repositories → services → handlers, registers routes, and returns the engine.
func Setup(db *sql.DB, jwtSecret string) *gin.Engine {
	r := gin.Default()
	r.Use(middleware.CORSMiddleware())

	// -- Repositories --
	riderRepo := repository.NewRiderRepository(db)
	orderRepo := repository.NewOrderRepository(db)
	assignmentRepo := repository.NewAssignmentRepository(db)
	earningsRepo := repository.NewEarningsRepository(db)
	notifRepo := repository.NewNotificationRepository(db)
	locationHistoryRepo := repository.NewLocationHistoryRepository(db)
	statusHistoryRepo := repository.NewStatusHistoryRepository(db)

	// -- Services --
	riderSvc := service.NewRiderService(riderRepo, orderRepo, earningsRepo)
	orderSvc := service.NewOrderService(orderRepo, assignmentRepo, riderRepo, earningsRepo, statusHistoryRepo)
	locationSvc := service.NewLocationService(riderRepo, locationHistoryRepo)
	earningsSvc := service.NewEarningsService(earningsRepo)
	notifSvc := service.NewNotificationService(notifRepo)

	// -- Handlers --
	healthH := handler.NewHealthHandler(db)
	uploadH := handler.NewUploadHandler()
	riderH := handler.NewRiderHandler(riderSvc)
	orderH := handler.NewOrderHandler(orderSvc)
	locationH := handler.NewLocationHandler(locationSvc)
	earningsH := handler.NewEarningsHandler(earningsSvc)
	notifH := handler.NewNotificationHandler(notifSvc)

	// ==================== PUBLIC ROUTES ====================
	r.GET("/health", healthH.Health)
	
	apiV1Public := r.Group("/api/v1")
	apiV1Public.POST("/upload", uploadH.HandleUpload)

	// ==================== PROTECTED ROUTES ====================
	auth := r.Group("/api/v1")
	auth.Use(middleware.AuthMiddleware(jwtSecret))

	// --- Rider Profile & Onboarding ---
	rider := auth.Group("/rider")
	{
		rider.GET("/profile", riderH.GetProfile)
		rider.PUT("/profile", riderH.UpdateProfile)
		rider.PUT("/vehicle", riderH.UpdateVehicle)
		rider.PUT("/bank-details", riderH.UpdateBankDetails)
		rider.PUT("/kyc", riderH.UpdateKYC)
		rider.GET("/onboarding-status", riderH.GetOnboardingStatus)
		rider.GET("/dashboard", riderH.GetDashboard)
		rider.POST("/go-online", riderH.GoOnline)
		rider.POST("/go-offline", riderH.GoOffline)
		rider.GET("/availability", riderH.GetAvailability)
	}

	// --- Orders & Assignments ---
	orders := auth.Group("/orders")
	{
		orders.GET("/available", orderH.GetAvailableOrders)
		orders.GET("/active", orderH.GetActiveOrder)
		orders.GET("/incoming", orderH.GetIncomingAssignment)
		orders.POST("/assignments/:id/accept", orderH.AcceptAssignment)
		orders.POST("/assignments/:id/reject", orderH.RejectAssignment)
		orders.GET("/:id", orderH.GetOrderDetail)
		orders.GET("/history", orderH.GetOrderHistory)
	}

	// --- Delivery Lifecycle ---
	delivery := auth.Group("/delivery")
	{
		delivery.POST("/:id/picked-up", orderH.PickedUp)
		delivery.POST("/:id/arrived-at-restaurant", orderH.ArrivedAtRestaurant)
		delivery.POST("/:id/arrived-at-customer", orderH.ArrivedAtCustomer)
		delivery.POST("/:id/delivered", orderH.Delivered)
		delivery.POST("/:id/cancel", orderH.CancelDelivery)
		delivery.POST("/:id/failed", orderH.FailDelivery)
	}

	// --- Location ---
	location := auth.Group("/location")
	{
		location.POST("/update", locationH.UpdateLocation)
		location.GET("/current", locationH.GetCurrentLocation)
	}

	// --- Earnings ---
	earnings := auth.Group("/earnings")
	{
		earnings.GET("/summary", earningsH.GetSummary)
		earnings.GET("/history", earningsH.GetHistory)
	}

	// --- Notifications ---
	notifications := auth.Group("/notifications")
	{
		notifications.POST("/device-token", notifH.RegisterDeviceToken)
		notifications.GET("", notifH.ListNotifications)
		notifications.PUT("/:id/read", notifH.MarkRead)
		notifications.PUT("/read-all", notifH.MarkAllRead)
		notifications.GET("/unread-count", notifH.GetUnreadCount)
	}

	return r
}
