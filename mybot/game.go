package mybot

import (
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var (
	gameState = make(map[int64]*PlayerSession)
	stateMu   sync.RWMutex

	globalGameState = &GlobalGameState{
		History: make([]float64, 0, 100),
	}
	Bot *tgbotapi.BotAPI
)

func RunGameLoop() {
	for {
		prepareNewRound()
		startRound()
		runRound()
		time.Sleep(pauseBetweenRounds)

		if globalGameState.CurrentRound%pauseEveryNRounds == 0 {
			broadcastMessage(fmt.Sprintf("🕐 Пауза на %d минут", pauseDuration/time.Minute))
			time.Sleep(pauseDuration)
		}
	}
}

func prepareNewRound() {
	stateMu.Lock()
	defer stateMu.Unlock()

	for chatID, session := range gameState {
		cleanupOldMessages(chatID, session)
		resetSession(session)
	}

	resetGlobalGameState()
	broadcastMessage(fmt.Sprintf("🆕 Новый раунд #%d\n\n⏳ Начало через %d секунд\n💰 Делайте ваши ставки!", globalGameState.CurrentRound, int(joinPeriod.Seconds())))
	time.Sleep(joinPeriod)
}

func resetSession(session *PlayerSession) {
	session.MessageIDs = nil
	session.CashedOut = false
	session.Multiplier = initialMultiplier
	session.InactiveRounds++

	if session.CashedOut {
		session.InactiveRounds = 0
	} else if session.InactiveRounds >= inactiveRoundLimit {
		session.Notifications = false
	}
}

func resetGlobalGameState() {
	globalGameState.CurrentRound++
	globalGameState.CurrentMultiplier = initialMultiplier
	globalGameState.TotalBets = 0
	globalGameState.ActivePlayers = 0
	globalGameState.CrashedMultiplier = 0
	globalGameState.RoundInProgress = false
	globalGameState.JoiningAllowed = true
}

func startRound() {
	stateMu.Lock()
	defer stateMu.Unlock()

	globalGameState.RoundStartTime = time.Now()
	globalGameState.RoundInProgress = true
	globalGameState.JoiningAllowed = false
}

func runRound() {
	ticker := time.NewTicker(updateInterval)
	defer ticker.Stop()

	speed := getRandomSpeed()

	for globalGameState.RoundInProgress {
		select {
		case <-ticker.C:
			stateMu.Lock()
			globalGameState.CurrentMultiplier += multiplierIncrease * speed

			if shouldEndRound() {
				endRound()
				stateMu.Unlock()
				return
			}

			updateAllPlayers()
			stateMu.Unlock()

			if rand.Float64() < speedChangeChance {
				speed = getRandomSpeed()
			}
		}
	}
}

func getRandomSpeed() float64 {
	return baseSpeed + rand.Float64()*(maxSpeed-baseSpeed)
}

func shouldEndRound() bool {
	if globalGameState.CurrentMultiplier >= maxMultiplier {
		return true
	}

	baseChance := 0.030
	multiplierFactor := (globalGameState.CurrentMultiplier - 1) / 200
	crashChance := baseChance + multiplierFactor

	return rand.Float64() < crashChance
}

func updateAllPlayers() {
	for chatID, session := range gameState {
		if !session.CashedOut {
			updatePlayerGame(chatID, session)
		}
	}
}

func generateField(x int) string {
	field := createEmptyField()

	for i := 0; i < x && i < fieldWidth && i < fieldHeight; i++ {
		field[fieldHeight-1-i][i] = "/"
	}

	rocketRow := fieldHeight - 1 - (x - 1)
	rocketCol := x - 1
	if rocketRow >= 0 && rocketRow < fieldHeight && rocketCol >= 0 && rocketCol < fieldWidth {
		field[rocketRow][rocketCol] = emojiRocket
	}

	placeMultiplier(field, globalGameState.CurrentMultiplier)

	return formatField(field)
}

func createEmptyField() [][]string {
	field := make([][]string, fieldHeight)
	for i := range field {
		field[i] = make([]string, fieldWidth)
		for j := range field[i] {
			field[i][j] = "•"
		}
	}
	return field
}

func placeMultiplier(field [][]string, multiplier float64) {
	coeffStr := fmt.Sprintf("x%.2f", multiplier)
	coeffRow := fieldHeight / 2
	coeffCol := fieldWidth - len(coeffStr) - 1
	for i, char := range coeffStr {
		field[coeffRow][coeffCol+i] = string(char)
	}
}

func formatField(field [][]string) string {
	var result strings.Builder
	for _, row := range field {
		result.WriteString(strings.Join(row, " ") + "\n")
	}
	return fmt.Sprintf("<pre>%s</pre>", result.String())
}

func updatePlayerGame(chatID int64, session *PlayerSession) {
	field := generateField(int(globalGameState.CurrentMultiplier * 2))
	winAmount := calculateWinAmount(session.Bet, globalGameState.CurrentMultiplier)

	keyboard := createGameKeyboard(session.CashedOut)

	newMessage := fmt.Sprintf("%s\n\n💰 Текущий выигрыш: %d\n📈 Множитель: x%.2f\n👥 Игроков: %d\n💵 Общая сумма ставок: %d\n\n🔥 Последние множители:\n%s",
		field, winAmount, globalGameState.CurrentMultiplier, globalGameState.ActivePlayers, globalGameState.TotalBets, formatHistory())

	if session.LastMessageText == newMessage && session.CashedOut == (len(keyboard.InlineKeyboard) == 0) {
		return
	}

	msg := tgbotapi.NewEditMessageText(chatID, session.LastMessageID, newMessage)
	msg.ReplyMarkup = &keyboard
	msg.ParseMode = "HTML"

	sentMsg, err := Bot.Send(msg)
	if err != nil && !strings.Contains(err.Error(), "Too Many Requests") {
		log.Printf("Error updating game message: %v", err)
	} else {
		session.LastMessageID = sentMsg.MessageID
		session.MessageIDs = append(session.MessageIDs, sentMsg.MessageID)
		session.LastMessageText = newMessage
	}
}

func createGameKeyboard(cashedOut bool) tgbotapi.InlineKeyboardMarkup {
	if cashedOut {
		return tgbotapi.NewInlineKeyboardMarkup()
	}
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("💰 Забрать выигрыш", "cashout"),
		),
	)
}

func formatHistory() string {
	history := globalGameState.History
	if len(history) > 5 {
		history = history[len(history)-5:]
	}
	result := make([]string, len(history))
	for i, mult := range history {
		result[i] = fmt.Sprintf("%.2fx", mult)
	}
	return strings.Join(result, " | ")
}

func endRound() {
	globalGameState.CrashedMultiplier = globalGameState.CurrentMultiplier
	globalGameState.History = append(globalGameState.History, globalGameState.CrashedMultiplier)
	globalGameState.RoundInProgress = false

	for chatID, session := range gameState {
		if !session.CashedOut {
			sendMessage(chatID, fmt.Sprintf("%s Бум! Раунд завершился при x%.2f", emojiExplosion, globalGameState.CrashedMultiplier))
		} else {
			broadcastMessage(fmt.Sprintf("🏁 Раунд #%d завершен\n\n💥 Крах при x%.2f\n📊 Статистика раунда:\n- Игроков: %d\n- Общая сумма ставок: %d",
				globalGameState.CurrentRound, globalGameState.CrashedMultiplier, globalGameState.ActivePlayers, globalGameState.TotalBets))
		}
	}
}

func handleStart(message *tgbotapi.Message) {
	text := "Добро пожаловать в игру! Используйте /bet для размещения ставки."
	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyMarkup = createBetKeyboard()
	Bot.Send(msg)
}

func createBetKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("/2", "bet / 2"),
			tgbotapi.NewInlineKeyboardButtonData("Повторить", "bet"),
			tgbotapi.NewInlineKeyboardButtonData("*2", "bet * 2"),
		),
			tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("50", "bet_50"),
			tgbotapi.NewInlineKeyboardButtonData("250", "bet_250"),
			tgbotapi.NewInlineKeyboardButtonData("500", "bet_500"),
		),
	)
}

func cashOut(chatID int64) {
	stateMu.Lock()
	defer stateMu.Unlock()

	session, exists := gameState[chatID]
	if !exists || session.CashedOut || !globalGameState.RoundInProgress {
		sendMessage(chatID, "Невозможно забрать выигрыш в данный момент.")
		return
	}

	winAmount := calculateWinAmount(session.Bet, globalGameState.CurrentMultiplier)
	session.CashedOut = true
	globalGameState.ActivePlayers--
	session.Balance += winAmount
	session.TotalWin += winAmount
	session.GamesPlayed++

	sendMessage(chatID, fmt.Sprintf("🎉 Поздравляем!\n\n💰 Вы забрали выигрыш: %d\n📈 Множитель: x%.2f\n💼 Ваш баланс: %d", winAmount, globalGameState.CurrentMultiplier, session.Balance))

	updatePlayerGame(chatID, session)
}

func calculateWinAmount(bet int, multiplier float64) int {
	return int(float64(bet) * multiplier * (1 - houseFee))
}

func sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"

	maxRetries := 5
	backoff := time.Second

	for i := 0; i < maxRetries; i++ {
		sentMsg, err := Bot.Send(msg)
		if err == nil {
			if session, exists := gameState[chatID]; exists {
				session.MessageIDs = append(session.MessageIDs, sentMsg.MessageID)
			}
			return
		}

		if strings.Contains(err.Error(), "Too Many Requests") || strings.Contains(err.Error(), "Bad Gateway") {
			log.Printf("Error sending message (attempt %d/%d): %v. Retrying...", i+1, maxRetries, err)
			time.Sleep(backoff)
			backoff *= 2
		} else {
			log.Printf("Error sending message: %v", err)
			return
		}
	}

	log.Printf("Failed to send message after %d attempts", maxRetries)
}

func broadcastMessage(text string) {
	for chatID, session := range gameState {
		if session.Notifications {
			sendMessage(chatID, text)
		}
	}
}

func cleanupOldMessages(chatID int64, session *PlayerSession) {
	for _, messageID := range session.MessageIDs {
		deleteMessage(chatID, messageID)
	}
}

func deleteMessage(chatID int64, messageID int) {
	msg := tgbotapi.NewDeleteMessage(chatID, messageID)
	if _, err := Bot.Request(msg); err != nil && !strings.Contains(err.Error(), "message to delete not found") {
		log.Printf("Error deleting message: %v", err)
	}
}
