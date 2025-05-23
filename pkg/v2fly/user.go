package v2fly

type User interface {
	GetEmail() string
	GetUUID() string
	GetAlterID() uint32
	GetLevel() uint32
}

var _ User = user{}

func NewUser(email, uuid string, alterID, level uint32) User {
	return user{
		email:   email,
		uuid:    uuid,
		alterID: alterID,
		level:   level,
	}
}

type user struct {
	email   string
	uuid    string
	alterID uint32
	level   uint32
}

func (u user) GetEmail() string {
	return u.email
}

func (u user) GetUUID() string {
	return u.uuid
}

func (u user) GetLevel() uint32 {
	return u.level
}

func (u user) GetAlterID() uint32 {
	return u.alterID
}
