package mybot

import (
	"time"
)

const (
	ApiToken           = "7336603620:AAHywDG0H_aTn91B3Km9NrU-LZo9oHW5WqQ"
	initialBalance     = 1000
	fieldWidth         = 20
	fieldHeight        = 15
	updateInterval     = 300 * time.Millisecond
	initialMultiplier  = 0.0
	multiplierIncrease = 0.05
	emojiRocket        = "🚀"
	emojiExplosion     = "💥"
	houseFee           = 0.05
	maxMultiplier      = 10.0
	minBet             = 5
	maxBet             = 1000
	joinPeriod         = 15 * time.Second
	pauseBetweenRounds = 5 * time.Second
	maxMessagesToKeep  = 5
	baseSpeed          = 1.5
	maxSpeed           = 3
	speedChangeChance  = 0.5
	pauseEveryNRounds  = 3
	pauseDuration      = 2 * time.Minute
	inactiveRoundLimit = 3
	maxRetryAttempts   = 3
	retryDelay         = 1 * time.Second
)

type PlayerSession struct {
	Bet             int
	Balance         int
	TotalWin        int
	GamesPlayed     int
	MessageIDs      []int
	LastMessageID   int
	CashedOut       bool
	InactiveRounds  int
	Notifications   bool
	Multiplier      float64
	LastMessageText string
}

type GlobalGameState struct {
	CurrentRound      int
	CurrentMultiplier float64
	TotalBets         int
	ActivePlayers     int
	CrashedMultiplier float64
	RoundInProgress   bool
	JoiningAllowed    bool
	History           []float64
	RoundStartTime    time.Time
}

func sendHelp(chatID int64) {
	helpText := `Доступные команды:
	/stats - показать вашу статистику
	/toggle_notifications - включить/выключить уведомления
	Чтобы сделать ставку, просто отправьте сумму в чат.`
	sendMessage(chatID, helpText)
}
