package users

import (
	"context"
	"testing"
)

func TestRegisterUser(t *testing.T) {
	//TODO: trancate the Users table (on stub database ofcourse). The table should be empty.
	usersData := []struct {
		login     string
		pwd       string
		firstName string
		lastName  string
		ok        bool
	}{
		{},                                              //invalid credentials
		{"+00000000000", "", "", "", false},             //invalid credentials
		{"", "Test", "", "", false},                     //invalid credentials
		{"", "", "Test", "", false},                     //invalid credentials
		{"", "", "", "Test", false},                     //invalid credentials
		{"+00000000001", "Test", "", "", false},         //invalid reg data
		{"+00000000002", "", "Test", "", false},         //invalid credentials
		{"+00000000003", "", "", "Test", false},         //invalid credentials
		{"", "Test", "Test", "", false},                 //invalid credentials
		{"", "Test", "", "Test", false},                 //invalid credentials
		{"", "", "Test", "Test", false},                 //invalid credentials
		{"+00000000004", "Test", "Test", "", true},      //OK
		{"+00000000005", "Test", "", "Test", false},     //invalid reg data
		{"+00000000006", "", "Test", "Test", false},     //invalid credentials
		{"", "Test", "Test", "Test", false},             //invalid credentials
		{"+00000000007", "Test", "Test", "Test", true},  //OK
		{"+00000000007", "Test", "Test", "Test", false}, //the user already exists
		{"Test", "Test", "Test", "Test", false},         //invalid login format//need another test
	}
	ctx := context.Background()
	for _, data := range usersData {
		user := new(User)
		err := user.Register(ctx, data.login, data.pwd, data.firstName, data.lastName)
		if data.ok && err != nil {
			t.Errorf("users: the user registration shouldn't executed with an error, but the error has occurred \"%s\". Data: %v", err, data)
		} else if !data.ok && err == nil {
			t.Errorf("users: the user registration should executed with an error, but the error hasn't occurred. Data: %v", data)
		}
	}
}

func TestAuthorizeUser(t *testing.T) {
	//TODO: trancate the Users table and create 1 user +00000000000 with pwd Test (on stub database ofcourse)
	usersData := []struct {
		login string
		pwd   string
		ok    bool
	}{
		{},                              //invalid credentials
		{"", "Test", false},             //invalid credentials
		{"+00000000000", "", false},     //invalid credentials
		{"00000000000", "Test", false},  //invalid login format
		{"+00000000000", "Test", true},  //OK
		{"+00000000001", "Test", false}} //the user does not exist
	ctx := context.Background()
	for _, data := range usersData {
		user := new(User)
		err := user.Authorize(ctx, data.login, data.pwd)
		if data.ok && err != nil {
			t.Errorf("users: the user authorization shouldn't executed with an error, but the error has occurred \"%s\". Data: %v", err, data)
		} else if !data.ok && err == nil {
			t.Errorf("users: the user authorization should executed with an error, but the error hasn't occurred. Data: %v", data)
		}
	}
}
