package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/arohanajit/user-service/middleware"

	"github.com/gin-gonic/gin"
	"github.com/hashicorp/consul/api"
	"github.com/joho/godotenv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func initConsul() (*api.Client, error) {
	config := api.DefaultConfig()
	config.Address = os.Getenv("CONSUL_HTTP_ADDR")
	if config.Address == "" {
		config.Address = "http://localhost:8500"
	}
	return api.NewClient(config)
}

func registerService(client *api.Client) error {
	port, _ := strconv.Atoi(os.Getenv("PORT"))
	registration := &api.AgentServiceRegistration{
		ID:      "user-service",
		Name:    "user-service",
		Port:    port,
		Address: "user-service",
		Check: &api.AgentServiceCheck{
			HTTP:                           fmt.Sprintf("http://user-service:%d/health", port),
			Interval:                       "10s",
			Timeout:                        "1s",
			DeregisterCriticalServiceAfter: "30s",
		},
		Tags: []string{"user", "api"},
	}
	return client.Agent().ServiceRegister(registration)
}

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file")
	}

	// Initialize database
	db, err := initDB()
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	// Auto migrate the schema
	if err := db.AutoMigrate(&User{}, &Address{}); err != nil {
		log.Fatal("Failed to migrate database:", err)
	}

	// Initialize Consul client
	consulClient, err := initConsul()
	if err != nil {
		log.Fatal("Failed to create Consul client:", err)
	}

	// Register service with Consul
	if err := registerService(consulClient); err != nil {
		log.Fatal("Failed to register service:", err)
	}

	// Initialize email service
	emailService := NewEmailService()

	// Initialize router
	r := gin.Default()

	// Health check endpoint
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	// Public routes
	r.POST("/register", Register(db))
	r.POST("/login", Login(db))
	r.POST("/forgot-password", RequestPasswordReset(db, emailService))
	r.POST("/reset-password", ResetPassword(db))

	// Protected routes
	protected := r.Group("/")
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "your-default-secret-key"
	}
	protected.Use(middleware.AuthMiddleware(jwtSecret))
	{
		// Profile management
		protected.GET("/profile", GetProfile(db))
		protected.PUT("/profile", UpdateProfile(db))
		protected.PUT("/profile/change-password", ChangePassword(db)) // Changed to POST
		protected.DELETE("/profile", DeleteAccount(db))

		// Address management
		protected.POST("/addresses", AddAddress(db))
		protected.GET("/addresses", ListAddresses(db))
		protected.PUT("/addresses/:id", UpdateAddress(db))
		protected.DELETE("/addresses/:id", DeleteAddress(db))
	}

	// Run the server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8002"
	}
	r.Run("0.0.0.0:" + port)
}

func initDB() (*gorm.DB, error) {
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
		os.Getenv("DB_PORT"),
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// Drop existing tables
	db.Migrator().DropTable(&Address{}, &User{})

	// Enable uuid-ossp extension
	db.Exec("CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\";")

	// Auto-migrate with new schema
	if err := db.AutoMigrate(&User{}, &Address{}); err != nil {
		return nil, err
	}

	return db, nil
}

// Additional helper functions...
