package router

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/client"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/config"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/handler"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/maps"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/middleware"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/repository"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/service"
	"github.com/Gursevak56/food-delivery-platform/services/rider-service/internal/ws"
)

// Setup creates all repositories → services → handlers, registers routes, and returns the engine.
func Setup(
	db *sql.DB,
	cfg *config.Config,
	hub *ws.Hub,
	deliverySvc *service.DeliveryService,
	restaurantCli *client.RestaurantClient,
	routeProviders ...maps.RouteProvider,
) *gin.Engine {
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
	deliveryRepo := repository.NewDeliveryRepository(db)

	// -- Services --
	riderSvc := service.NewRiderService(riderRepo, orderRepo, earningsRepo)
	orderSvc := service.NewOrderService(orderRepo, deliveryRepo, assignmentRepo, riderRepo, earningsRepo, statusHistoryRepo, restaurantCli)
	orderSvc.SetDeliveryService(deliverySvc)
	locationSvc := service.NewLocationService(riderRepo, locationHistoryRepo)
	// The app's location route feeds the Redis dispatch index too.
	locationSvc.SetLocationIndexer(deliverySvc)
	locationSvc.SetLocationNotifier(deliverySvc)
	earningsSvc := service.NewEarningsService(earningsRepo)
	notifSvc := service.NewNotificationService(notifRepo)
	var deliveryRouteSvc *service.DeliveryRouteService
	if len(routeProviders) > 0 && routeProviders[0] != nil {
		deliveryRouteSvc = service.NewDeliveryRouteService(orderSvc, routeProviders[0], service.DeliveryRouteOptions{
			TrafficAware: cfg.GoogleMaps.RoutesTrafficEnabled,
			ResultTTL:    cfg.GoogleMaps.RouteCacheTTL,
		})
	}

	// -- Handlers --
	healthH := handler.NewHealthHandler(db)
	uploadH := handler.NewUploadHandler()
	riderH := handler.NewRiderHandler(riderSvc)
	orderH := handler.NewOrderHandler(orderSvc)
	locationH := handler.NewLocationHandler(locationSvc)
	earningsH := handler.NewEarningsHandler(earningsSvc, db)
	notifH := handler.NewNotificationHandler(notifSvc)
	deliveryH := handler.NewDeliveryHandler(deliverySvc, locationSvc)
	deliveryH.SetRouteService(deliveryRouteSvc)
	adminRiderH := handler.NewAdminRiderHandler(db)
	supportH := handler.NewSupportHandler(db)

	// ==================== PUBLIC ROUTES ====================
	r.GET("/health", healthH.Health)

	// ==================== WEBSOCKET ROUTES ====================
	// Rider WebSocket uses token from query or auth header
	r.GET("/ws/rider", hub.HandleRiderWS(cfg.JWTSecret))
	// Tracking WebSocket for customer app
	r.GET("/ws/tracking/orders/:orderId", middleware.TrackingToken(), middleware.AuthMiddleware(cfg.JWTSecret), middleware.TrackingAccess(db), hub.HandleOrderTrackingWS())

	internal := r.Group("/internal")
	internal.Use(middleware.RequireInternalServiceToken(cfg.InternalServiceToken))
	{
		internal.GET("/tracking/orders/:orderId", deliveryH.GetInternalCustomerTracking)
	}

	// ==================== PROTECTED ROUTES ====================
	auth := r.Group("/api/v1")
	auth.Use(middleware.AuthMiddleware(cfg.JWTSecret))

	// Document/photo upload: authenticated riders only (was public).
	auth.POST("/upload", uploadH.HandleUpload)

	// --- Rider Profile & Onboarding ---
	rider := auth.Group("/rider")
	{
		rider.GET("/profile", riderH.GetProfile)
		rider.PUT("/profile", riderH.UpdateProfile)
		rider.DELETE("/account", riderH.DeleteAccount)
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

	// --- NEW Delivery Lifecycle ---
	newDelivery := auth.Group("/riders")
	{
		newDelivery.POST("/location", deliveryH.UpdateLocation)
		newDelivery.POST("/availability", deliveryH.UpdateAvailability)
		newDelivery.GET("/order-requests", deliveryH.GetOrderRequests)
		newDelivery.POST("/order-requests/:requestId/accept", deliveryH.AcceptRequest)
		newDelivery.POST("/order-requests/:requestId/reject", deliveryH.RejectRequest)
		newDelivery.GET("/orders", deliveryH.GetRiderOrders)
		newDelivery.GET("/orders/:orderId", deliveryH.GetRiderOrderDetail)
		newDelivery.POST("/orders/:orderId/route", deliveryH.GetDeliveryRoute)
		newDelivery.POST("/orders/:orderId/status", deliveryH.UpdateDeliveryStatus)
		newDelivery.POST("/orders/:orderId/withdraw", deliveryH.WithdrawDelivery)
	}

	// --- NEW Delivery Tracking (For Customer App, internal or authenticated) ---
	r.GET("/api/v1/delivery/orders/:orderId/tracking", middleware.AuthMiddleware(cfg.JWTSecret), middleware.TrackingAccess(db), deliveryH.GetDeliveryTracking)

	// --- Earnings ---
	earnings := auth.Group("/earnings")
	{
		earnings.GET("/summary", earningsH.GetSummary)
		earnings.GET("/history", earningsH.GetHistory)
		// Self-service wallet ledger (platform upgrade Module 21) — same
		// data Module 10's admin view reads, scoped to the caller's own ID.
		earnings.GET("/wallet/transactions", earningsH.GetWalletTransactions)
		earnings.GET("/wallet/settlements", earningsH.GetSettlements)
	}

	// --- Admin: Rider Management (platform upgrade Module 10/11) ---
	adminRiders := auth.Group("/admin/riders")
	adminRiders.Use(middleware.RequireAdmin())
	{
		adminRiders.GET("", adminRiderH.ListRiders)
		adminRiders.GET("/:riderId/wallet/transactions", adminRiderH.GetWalletTransactions)
		adminRiders.GET("/:riderId/settlements", adminRiderH.GetSettlements)
		adminRiders.POST("/:riderId/settlements", adminRiderH.CreateSettlement)
		adminRiders.POST("/:riderId/wallet/adjustments", adminRiderH.CreateWalletAdjustment)
	}

	// --- Admin: Reports and Exports (platform upgrade Module 24) ---
	adminRiderReports := auth.Group("/admin/reports/riders")
	adminRiderReports.Use(middleware.RequireAdmin())
	{
		adminRiderReports.GET("/wallet-transactions/export-csv", adminRiderH.ExportWalletTransactionsCSV)
		adminRiderReports.GET("/settlements/export-csv", adminRiderH.ExportSettlementsCSV)
	}

	// --- Rider support: tickets, contact details, and the admin queue ---
	support := auth.Group("/support")
	{
		support.POST("/tickets", supportH.CreateTicket)
		support.GET("/tickets", supportH.ListMyTickets)
		support.GET("/contact", supportH.Contact)
	}
	adminSupport := auth.Group("/admin/support-tickets")
	adminSupport.Use(middleware.RequireAdmin())
	{
		adminSupport.GET("", supportH.AdminListTickets)
		adminSupport.PATCH("/:id", supportH.AdminUpdateTicket)
	}

	// --- Notifications ---
	notifications := auth.Group("/notifications")
	{
		notifications.POST("/device-token", notifH.RegisterDeviceToken)
		notifications.DELETE("/device-token", notifH.UnregisterDeviceToken)
		notifications.GET("", notifH.ListNotifications)
		notifications.PUT("/:id/read", notifH.MarkRead)
		notifications.PUT("/read-all", notifH.MarkAllRead)
		notifications.GET("/unread-count", notifH.GetUnreadCount)
	}

	return r
}
