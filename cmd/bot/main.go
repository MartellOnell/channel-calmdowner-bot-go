package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"channel-calmdowner-bot/internal/bot"
	"channel-calmdowner-bot/internal/config"
	"channel-calmdowner-bot/internal/storage"
	"channel-calmdowner-bot/internal/telegram"
)

func main() {
	cfg := config.Load()
	if cfg.BotToken == "" {
		log.Fatal("BOT_TOKEN environment variable is required")
	}

	botID := config.ExtractBotID(cfg.BotToken)
	if botID == 0 {
		log.Fatal("Cannot extract bot ID from BOT_TOKEN")
	}
	log.Printf("Bot ID: %d", botID)

	store, err := storage.New(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer store.Close()

	tg := telegram.NewClient(cfg.BotToken)
	b := bot.New(botID, tg, store)

	go b.StartPeriodicCheck(cfg.CheckInterval)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	log.Println("Bot started -- listening for messages")
	go b.StartPolling()

	<-stop
	log.Println("Shutting down...")
}
