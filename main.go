package main

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func init() {
	// Load .env silently (no error if file missing)
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, relying on environment")
	}
}

func main() {
	r := gin.Default()

	InitDB()
	// Health check endpoint
	r.GET("/health", func(c *gin.Context) {
		// Remove the undefined healthCheckHandler call
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Example user endpoints
	r.POST("/users", createUser)
	r.GET("/users/:id", getUser)
	r.PUT("/users/:id", updateUser)
	r.DELETE("/users/:id", deleteUser)

	r.Run(":8084")
}

func createUser(c *gin.Context) {
	// TODO: business logic
	c.JSON(http.StatusCreated, gin.H{"message": "user created"})
}

func getUser(c *gin.Context) {
	// TODO: business logic
	c.JSON(http.StatusOK, gin.H{"id": c.Param("id"), "name": "example"})
}

func updateUser(c *gin.Context) {
	// TODO: business logic
	c.JSON(http.StatusOK, gin.H{"message": "user updated"})
}

func deleteUser(c *gin.Context) {
	// TODO: business logic
	c.JSON(http.StatusOK, gin.H{"message": "user deleted"})
}
