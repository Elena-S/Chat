package users

import (
	"context"
	"testing"
)

func TestRegisterUser(t *testing.T) {
	//TODO: need mock repository
	manager := NewManager(ManagerParams{}) //dummy
	RegData := []struct {
		RegData
		ok bool
	}{
		{}, //invalid credentials
		{RegData{"+00000000000", "", "", ""}, false},             //invalid credentials
		{RegData{"", "Test", "", ""}, false},                     //invalid credentials
		{RegData{"", "", "Test", ""}, false},                     //invalid credentials
		{RegData{"", "", "", "Test"}, false},                     //invalid credentials
		{RegData{"+00000000001", "Test", "", ""}, false},         //invalid reg data
		{RegData{"+00000000002", "", "Test", ""}, false},         //invalid credentials
		{RegData{"+00000000003", "", "", "Test"}, false},         //invalid credentials
		{RegData{"", "Test", "Test", ""}, false},                 //invalid credentials
		{RegData{"", "Test", "", "Test"}, false},                 //invalid credentials
		{RegData{"", "", "Test", "Test"}, false},                 //invalid credentials
		{RegData{"+00000000004", "Test", "Test", ""}, true},      //OK
		{RegData{"+00000000005", "Test", "", "Test"}, false},     //invalid reg data
		{RegData{"+00000000006", "", "Test", "Test"}, false},     //invalid credentials
		{RegData{"", "Test", "Test", "Test"}, false},             //invalid credentials
		{RegData{"+00000000007", "Test", "Test", "Test"}, true},  //OK
		{RegData{"+00000000007", "Test", "Test", "Test"}, false}, //the user already exists
		{RegData{"Test", "Test", "Test", "Test"}, false},         //invalid login format//need another test
	}
	ctx := context.Background()
	for _, data := range RegData {
		_, err := manager.Register(ctx, data.RegData)
		if data.ok && err != nil {
			t.Errorf("users: the user registration shouldn't executed with an error, but the error has occurred \"%s\". Data: %v", err, data)
		} else if !data.ok && err == nil {
			t.Errorf("users: the user registration should executed with an error, but the error hasn't occurred. Data: %v", data)
		}
	}
}

func TestAuthorizeUser(t *testing.T) {
	//TODO: need mock repository
	manager := NewManager(ManagerParams{}) //dummy
	usersAuthData := []struct {
		RegData
		ok bool
	}{
		{},                                   //invalid credentials
		{RegData{"", "Test", "", ""}, false}, //invalid credentials
		{RegData{"+00000000000", "", "", ""}, false},     //invalid credentials
		{RegData{"00000000000", "Test", "", ""}, false},  //invalid login format
		{RegData{"+00000000000", "Test", "", ""}, true},  //OK
		{RegData{"+00000000001", "Test", "", ""}, false}} //the user does not exist
	ctx := context.Background()
	for _, data := range usersAuthData {
		_, err := manager.Authorize(ctx, data.RegData)
		if data.ok && err != nil {
			t.Errorf("users: the user authorization shouldn't executed with an error, but the error has occurred \"%s\". Data: %v", err, data)
		} else if !data.ok && err == nil {
			t.Errorf("users: the user authorization should executed with an error, but the error hasn't occurred. Data: %v", data)
		}
	}
}
