package routs

import (
	"net/http"

	"github.com/Elena-S/Chat/pkg/handlers"
	"golang.org/x/net/websocket"
)

func SetupRouts() {
	http.HandleFunc("/", handlers.Home)
	http.HandleFunc("/error", handlers.Error)
	http.HandleFunc("/authentication/login", handlers.Login)
	http.HandleFunc("/authentication/consent", handlers.Consent)
	http.HandleFunc("/authentication/logout", handlers.Logout)
	http.HandleFunc("/authentication/finish", handlers.FinishAuth)
	http.HandleFunc("/authentication/refresh_tokens", handlers.RefreshTokens)
	http.HandleFunc("/chat", handlers.Chat)
	http.HandleFunc("/chat/search", handlers.Search)
	http.HandleFunc("/chat/list", handlers.ChatList)
	http.HandleFunc("/chat/history", handlers.ChatHistory)
	http.HandleFunc("/chat/create", handlers.CreateChat)
	http.HandleFunc("/chat/chat", handlers.ChatAbout)
	http.HandleFunc("/chat/user", handlers.UserAbout)
	http.Handle("/chat/ws", websocket.Handler(handlers.SendMessage))

	//NGINX
	http.Handle("/view/", http.FileServer(http.Dir("/usr/src/app")))
}
