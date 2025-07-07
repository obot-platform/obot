package types

type APIActivity struct {
	UserID string `json:"userID"`
	Date   Time   `json:"date"`
}

type APIActivityList List[APIActivity]
