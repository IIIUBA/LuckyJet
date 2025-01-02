package mybot

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func showPlayerStats(chatID int64) {
	stateMu.RLock()
	defer stateMu.RUnlock()

	session, exists := gameState[chatID]
	if !exists {
		sendMessage(chatID, "У вас нет игровой статистики.")
		return
	}

	stats := fmt.Sprintf("📊 Ваша статистика:\n"+
		"Текущая ставка: %d\n"+
		"Ваш баланс: %d\n"+
		"Активных раундов: %d\n"+
		"Уведомления: %v",
		session.Bet,
		session.Balance,
		inactiveRoundLimit-session.InactiveRounds,
		session.Notifications)

	sendMessage(chatID, stats)
}

func toggleNotifications(chatID int64) {
	stateMu.Lock()
	defer stateMu.Unlock()

	session, exists := gameState[chatID]
	if !exists {
		session = &PlayerSession{
			MessageIDs:    make([]int, 0, maxMessagesToKeep),
			Notifications: true,
		}
		gameState[chatID] = session
	}

	session.Notifications = !session.Notifications
	status := "включены"
	if !session.Notifications {
		status = "выключены"
	}
	sendMessage(chatID, fmt.Sprintf("Уведомления %s.", status))
}

func HandleCallback(callback *tgbotapi.CallbackQuery) {
	chatID := callback.Message.Chat.ID

	switch callback.Data {
	case "bet / 2":
		handleHalfBet(chatID)
	case "bet":
		handleRepeatBet(chatID)
	case "bet * 2":
		handleDoubleBet(chatID)
	case "cashout":
		cashOut(chatID)
	default:
		if strings.HasPrefix(callback.Data, "bet_") {
			handleFixedBet(chatID, callback.Data)
		} else {
			sendMessage(chatID, "Неизвестная команда.")
		}
	}

	callbackCfg := tgbotapi.NewCallback(callback.ID, "")
	if _, err := Bot.Request(callbackCfg); err != nil {
		log.Printf("Ошибка при ответе на callback: %v", err)
	}
}

func HandleMessage(message *tgbotapi.Message) {
	if message.IsCommand() {
		handleCommand(message)
		return
	}

	
	if !globalGameState.JoiningAllowed {
		sendMessage(message.Chat.ID, "Извините, сейчас нельзя делать ставки. Дождитесь следующего раунда.")
		return
	}

	bet, err := strconv.Atoi(message.Text)
	if err != nil || bet < minBet || bet > maxBet {
		sendMessage(message.Chat.ID, fmt.Sprintf("Пожалуйста, введите корректную ставку от %d до %d.", minBet, maxBet))
		return
	}

	placeBet(message.Chat.ID, bet)
}

func handleCommand(message *tgbotapi.Message) {
	switch message.Command() {
	case "start":
		handleStart(message)
	case "help":
		sendHelp(message.Chat.ID)
	case "stats":
		showPlayerStats(message.Chat.ID)
	case "toggle_notifications":
		toggleNotifications(message.Chat.ID)
	case "bet":
		handleBetCommand(message)
	default:
		sendMessage(message.Chat.ID, "Неизвестная команда. Используйте /help для получения списка команд.")
	}
}

func placeBet(chatID int64, betAmount int) {
	stateMu.Lock()
	defer stateMu.Unlock()

	session, exists := gameState[chatID]
	if !exists {
		session = &PlayerSession{
			MessageIDs:    make([]int, 0, maxMessagesToKeep),
			Notifications: true,
			Balance:       initialBalance,
		}
		gameState[chatID] = session
	}

	if betAmount < minBet || betAmount > maxBet {
		sendMessage(chatID, fmt.Sprintf("Ставка должна быть между %d и %d.", minBet, maxBet))
		return
	}

	if session.Balance < betAmount {
		sendMessage(chatID, fmt.Sprintf("Недостаточно средств. Ваш баланс: %d", session.Balance))
		return
	}

	if session.Bet > 0 {
		session.Balance += session.Bet
		globalGameState.TotalBets -= session.Bet
		globalGameState.ActivePlayers--
	}

	session.Balance -= betAmount
	session.Bet = betAmount
	session.CashedOut = false
	session.Multiplier = initialMultiplier
	session.InactiveRounds = 0
	globalGameState.TotalBets += betAmount
	globalGameState.ActivePlayers++

	msg := fmt.Sprintf("Ваша ставка %d принята. Ваш баланс: %d. Ожидайте начала раунда!", betAmount, session.Balance)
	sendMessage(chatID, msg)

	updatePlayerMessage(chatID, session)
}

func handleBetCommand(message *tgbotapi.Message) {
	args := message.CommandArguments()
	if args == "" {
		text := "Пожалуйста, укажите сумму ставки. Например: 100"
		msg := tgbotapi.NewMessage(message.Chat.ID, text)
		msg.ReplyMarkup = createBetKeyboard()
		Bot.Send(msg)
		return
	}

	bet, err := strconv.Atoi(args)
	if err != nil {
		sendMessage(message.Chat.ID, "Пожалуйста, введите корректное число для ставки.")
		return
	}

	placeBet(message.Chat.ID, bet)
}

func handleFixedBet(chatID int64, callbackData string) {
	betAmount, err := strconv.Atoi(strings.TrimPrefix(callbackData, "bet_"))
	if err != nil {
		sendMessage(chatID, "Произошла ошибка при обработке ставки.")
		return
	}

	placeBet(chatID, betAmount)
}

func handleHalfBet(chatID int64) {
	stateMu.RLock()
	session, exists := gameState[chatID]
	stateMu.RUnlock()

	if !exists || session.Bet == 0 {
		sendMessage(chatID, "Невозможно уменьшить ставку. Сначала сделайте ставку.")
		return
	}

	newBet := session.Bet / 2
	if newBet < minBet {
		sendMessage(chatID, fmt.Sprintf("Минимальная ставка %d. Невозможно уменьшить текущую ставку.", minBet))
		return
	}

	placeBet(chatID, newBet)
}

func handleRepeatBet(chatID int64) {
	stateMu.RLock()
	session, exists := gameState[chatID]
	stateMu.RUnlock()

	if !exists || session.Bet == 0 {
		sendMessage(chatID, "Нет предыдущей ставки для повтора. Сделайте новую ставку.")
		return
	}

	placeBet(chatID, session.Bet)
}

func handleDoubleBet(chatID int64) {
	stateMu.RLock()
	session, exists := gameState[chatID]
	stateMu.RUnlock()

	if !exists || session.Bet == 0 {
		sendMessage(chatID, "Невозможно удвоить ставку. Сначала сделайте ставку.")
		return
	}

	newBet := session.Bet * 2
	if newBet > maxBet {
		sendMessage(chatID, fmt.Sprintf("Максимальная ставка %d. Невозможно увеличить текущую ставку.", maxBet))
		return
	}

	placeBet(chatID, newBet)
}

func updatePlayerMessage(chatID int64, session *PlayerSession) {
	text := fmt.Sprintf("Ваша ставка: %d\nВаш баланс: %d", session.Bet, session.Balance)

	if session.LastMessageID == 0 {
		msg := tgbotapi.NewMessage(chatID, text)
		sentMsg, err := Bot.Send(msg)
		if err != nil {
			log.Printf("Error sending new message: %v", err)
			return
		}
		session.LastMessageID = sentMsg.MessageID
	} else {
		msg := tgbotapi.NewEditMessageText(chatID, session.LastMessageID, text)

		_, err := Bot.Send(msg)
		if err != nil {
			if strings.Contains(err.Error(), "message to edit not found") {
				newMsg := tgbotapi.NewMessage(chatID, text)
				sentMsg, err := Bot.Send(newMsg)
				if err != nil {
					log.Printf("Error sending new message: %v", err)
					return
				}
				session.LastMessageID = sentMsg.MessageID
			} else {
				log.Printf("Error updating message: %v", err)
			}
		}
	}
}
