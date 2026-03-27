package tictactoe

import "fmt"

type Mode string

const (
	ModeClassic Mode = "classic"
	ModeTimed   Mode = "timed"
)

type Status string

const (
	StatusWaiting    Status = "waiting"
	StatusInProgress Status = "in_progress"
	StatusFinished   Status = "finished"
)

type Game struct {
	Board        [9]int
	TurnSymbol   int
	WinnerSymbol int
}

type MoveResult struct {
	WinnerSymbol int
	Draw         bool
	NextTurn     int
}

func NewGame() Game {
	return Game{TurnSymbol: 1}
}

func NormalizeMode(m string) Mode {
	if m == string(ModeTimed) {
		return ModeTimed
	}
	return ModeClassic
}

func (g *Game) ApplyMove(cell int, symbol int) (MoveResult, error) {
	if cell < 0 || cell > 8 {
		return MoveResult{}, fmt.Errorf("cell must be between 0 and 8")
	}
	if symbol != 1 && symbol != 2 {
		return MoveResult{}, fmt.Errorf("invalid symbol")
	}
	if g.TurnSymbol != symbol {
		return MoveResult{}, fmt.Errorf("not your turn")
	}
	if g.Board[cell] != 0 {
		return MoveResult{}, fmt.Errorf("cell already occupied")
	}

	g.Board[cell] = symbol
	if winner := checkWinner(g.Board); winner != 0 {
		g.WinnerSymbol = winner
		return MoveResult{WinnerSymbol: winner}, nil
	}
	if boardFull(g.Board) {
		return MoveResult{Draw: true}, nil
	}

	if symbol == 1 {
		g.TurnSymbol = 2
	} else {
		g.TurnSymbol = 1
	}

	return MoveResult{NextTurn: g.TurnSymbol}, nil
}

func checkWinner(board [9]int) int {
	lines := [8][3]int{
		{0, 1, 2},
		{3, 4, 5},
		{6, 7, 8},
		{0, 3, 6},
		{1, 4, 7},
		{2, 5, 8},
		{0, 4, 8},
		{2, 4, 6},
	}
	for _, line := range lines {
		a, b, c := line[0], line[1], line[2]
		if board[a] != 0 && board[a] == board[b] && board[b] == board[c] {
			return board[a]
		}
	}
	return 0
}

func boardFull(board [9]int) bool {
	for _, cell := range board {
		if cell == 0 {
			return false
		}
	}
	return true
}
