package routs

import (
	"net/http"

	"github.com/Elena-S/Chat/pkg/handlers"
	"golang.org/x/net/websocket"
)

func SetupRouts() {
	http.HandleFunc("/authorization", handlers.Authorize)
	http.HandleFunc("/registration", handlers.Register)
	http.HandleFunc("/chat/search", handlers.Search)
	http.HandleFunc("/chat/list", handlers.ChatList)
	http.HandleFunc("/chat/history", handlers.ChatHistory)
	http.HandleFunc("/chat/create", handlers.CreateChat)
	http.HandleFunc("/chat/user", handlers.UserAbout)
	http.Handle("/chat/ws", websocket.Handler(handlers.SendMessage))

	//NGINX
	fsHandler := &handlers.FsAccess{
		Handler: http.FileServer(http.Dir("/usr/src/app/web")),
	}
	http.Handle("/", fsHandler)
}
