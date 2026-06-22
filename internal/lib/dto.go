package lib

import (
	db "aic3-service/db/postgres/sqlc"
)

func ToUserResponse(u db.SelectUserByLoginIdWithPasswordRow) db.UsersView {
	return db.UsersView{
		ID:        u.ID,
		Name:      u.Name,
		Username:  u.Username,
		Email:     u.Email,
		Role:      u.Role,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}
