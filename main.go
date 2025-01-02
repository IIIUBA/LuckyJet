package main

import (
	"log"
	"math/rand"
	"mybot/mybot"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	var err error
	mybot.Bot, err = tgbotapi.NewBotAPI(mybot.ApiToken)
	if err != nil {
		log.Panic(err)
	}

	go mybot.RunGameLoop()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := mybot.Bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			go mybot.HandleMessage(update.Message)
		} else if update.CallbackQuery != nil {
			go mybot.HandleCallback(update.CallbackQuery)
		}
	}


	
}
