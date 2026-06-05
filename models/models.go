package models

type UserRequest struct {
	Name      string `form:"username"`
	Email     string `form:"email"`
	Password  string `form:"password"`
	Role      string `form:"role"`
	Faculty   string `form:"faculty"`
	EntryYear string `form:"entryYear"`
	Bio       string `form:"bio"`
}
