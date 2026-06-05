package models

type UserRequest struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	Role      string `json:"role"`
	Faculty   string `json:"faculty"`
	EntryYear string `json:"entryYear"`
	Bio       string `json:"bio"`
}
