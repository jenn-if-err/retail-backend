package main

import (
	"context"
	"io/ioutil"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/jenn-if-err/retail-backend/internal/db"
	"github.com/jenn-if-err/retail-backend/internal/handlers"
	"gopkg.in/yaml.v3"
)

type Config struct {
	SpannerDB string `yaml:"spannerDb"`
}

func main() {
	configData, err := ioutil.ReadFile("config.yaml")
	if err != nil {
		log.Fatalf("failed to read config.yaml: %v", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(configData, &cfg); err != nil {
		log.Fatalf("failed to parse config.yaml: %v", err)
	}
	if cfg.SpannerDB == "" {
		log.Fatal("spanner_db not set in config.yaml")
	}
	ctx := context.Background()
	spannerClient, err := db.NewSpannerClient(ctx, cfg.SpannerDB)
	if err != nil {
		log.Fatalf("failed to create spanner client: %v", err)
	}
	defer spannerClient.Close()

	r := gin.Default()
	r.POST("/checkout", handlers.CheckoutHandler(spannerClient))

	if err := r.Run(); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
