

window.addEventListener('load', () => {
    const chatManager = new ChatView();
});

class ChatView {
    #currentChat;
    #ws;

    #onMessage(event) {
        const rawMessage = JSON.parse(event.data);
        const chat = this._chatList.get(rawMessage.ChatID);
        if (rawMessage.Service) {
            if (chat.id() != this.#currentChat.id() || user.id() == rawMessage.AuthorID) {
                return;
            } 
            this.#notify(rawMessage);
        } else {
            const message = chat.addMessage(rawMessage);
            
            if (chat.id() == this.#currentChat.id()) {
                this.#displayMessageInHistory(this._historyElement.innerHTML + message.html());
            }

            this._chatList.refreshChatView(chat);
            this._chatList.upElement(chat);
        }
    }

    async #onSend(event, text) {
        let chat = this.#currentChat;
 
        if (!chat) {
            return;
        }

        let isService = !!(text);
        if (!isService) {
            text = this._inputElement.value;
            this._inputElement.value = '';
        }

        if (!text) {
            return;
        }

        if (!chat.id()) {
            if (isService) {
                return;
            }
            chat = await Chat.create(chat);
            this._chatList.add(chat);
            this.setCurrentChat(chat, false);
        }

        if (this.#ws.readyState > 1) {
            await this.newWebSocketConn(); 
        }
        
        chat.sendMessage(this.#ws, text, isService);

    }

    #onKeyDown(event) {
        if (event.key == "Enter") {
            event.preventDefault();
            this.#onSend(event);
        }
    }
    
    #onTypingText = flushAndThrottle(async (event) => {
        this.#onSend(event, 'typing text...');
    }, 1000, true);

    #notify = function() {
        let id;
        return (rawMessage) => {
                clearTimeout(id);
                this._notifier.innerHTML = rawMessage.Text;
                this._notifier.classList.remove('invisible');
                id = setTimeout(() => {
                        if (!this._notifier.classList.contains('invisible')) {
                            this._notifier.classList.add('invisible');
                        }
                        this._notifier.innerHTML = '';
                    }, 1100);
            };
    }();

    #onChoose(chats, event) {
        for (let item = event.target; item != event.currentTarget; item = item.parentElement) {
            if (item.tagName == 'LI') {
                const chat = this.#currentChat;
                if (chat) {
                    chat.saveInputText(this._inputElement.value);
                    this._chatList.unmarkChatAsCurrent(chat);
                }
                
                const id = Number(item.getAttribute(Chat.metaProperty));
                this.setCurrentChat(chats.get(id));
               
                break;
            }
        }
        this._chatList.show();
        this._searchList.hide();
    }

    #onSearchEnd(){
        if (this._searchList.emptyInput()) {
            this._chatList.show();
            this._searchList.hide();
        } else {
            this._chatList.hide();
            this._searchList.show();
        }
    }

    #onScrollHistory = throttle(async (event) => {
        const height = this._historyElement.scrollHeight;

        if (event.target.scrollTop <= event.target.clientHeight*2) {
            const messages = await this.#currentChat.updateHistory();
            const html = this.#fillHistory(messages);
            this._historyElement.innerHTML = html + this._historyElement.innerHTML;
            event.target.scrollTop += this._historyElement.scrollHeight - height; 
        }
    }, 100);

    constructor(chat) {
        //data
        this.#currentChat = chat;
        this._chatList = new ChatList(this.#onChoose.bind(this),);
        this._searchList = new SearchList(this.#onChoose.bind(this),  this.#onSearchEnd.bind(this));

        //view
        this._historyElement = document.getElementById('chat-history');
        this._historyElement.addEventListener('scroll', this.#onScrollHistory.bind(this));

        this._headerElement = document.getElementById('chat-header'); 

        this._notifier = document.getElementById('chat-notification');

        this._inputElement = document.getElementById('chat-message');
        this._inputElement.addEventListener('keydown', this.#onKeyDown.bind(this));
        this._inputElement.addEventListener('input', this.#onTypingText.bind(this));

        this._buttonElement = document.getElementById('send-message');
        this._buttonElement.addEventListener('click', this.#onSend.bind(this));

        //action
        this.newWebSocketConn();
        this._chatList.update();
    }
    
    async newWebSocketConn() {
        this.#ws = new WebSocket(`ws://${ host }/chat/ws`);
        this.#ws.addEventListener('message', this.#onMessage.bind(this)); 
        while (this.#ws.readyState < 1) {
            await sleep(100);
        }   
    }

    setCurrentChat(chat, restoreHistory = true) {
        this.#currentChat = chat;
        this._chatList.markChatAsCurrent(chat);
        this.refreshHeader();
        this._inputElement.value = chat.getSavedInputText();
        
        if (restoreHistory) {
            this.restoreHistory();
        }

        let event = new Event('scroll', { bubbles: true });
        this._historyElement.dispatchEvent(event);
    }

    refreshHeader() {
        if (this.#currentChat) {
            this._headerElement.innerHTML = `${ this.#currentChat.presentation() } <div> ${ this.#currentChat.phone() } </div>`;
        } else {
            this._headerElement.innerHTML = '';
        }
    }

    async restoreHistory() {
        const messages = await this.#currentChat.messages();
        const html = this.#fillHistory(messages);
        this.#displayMessageInHistory(html);
    }

    #fillHistory(messages) {
        let html = '';
        messages.forEach((message) => {
            html = message.html() + html;
        });
        return html;
    }

    #displayMessageInHistory(html) {
        this._historyElement.innerHTML = html;
        this._historyElement.scrollTop = this._historyElement.scrollHeight;
    }
    
}

class User {
    #phone;
    #firstName;
    #lastName;
    #fullName;
    constructor() {
        fetchJSON('chat/user', (data) => {
            this._id = data.ID;
            this.#phone=  data.Phone;
            this.#firstName = data.FirstName;
            this.#lastName = data.LastName;
            this.#fullName = data.FullName;
        });
    }

    id() {
        return this._id;
    }
}

class ChatList {
    #chats;

    constructor(onclick) {
        this.#chats = new Map;
        this._element = document.getElementById('chat-list');
        this._element.addEventListener('click', onclick.bind(undefined, this));
    }

    newElement(chat) {
        return `<li class="dialog-chatlist-item dialog-commo" ${ Chat.metaProperty }="${chat.id()}">${this.elementView(chat)}</li>`;
    }

    elementView(chat) {
        let strData = chat.lastMessageDate().toLocaleString('ru', {
            month: 'short',
            day: 'numeric'
        });
        return `${chat.presentation()}<span> ${strData}</span><div>${chat.lastMessageText().slice(0,30)}</div>`;  
    }

    update() {
        this._element.innerHTML = '';
        fetchJSON(`chat/list`, (chats) => {
            if (!chats) {
                return;
            }
            this.load(chats);
            if (this.#chats.size > 0 && this._element.firstChild) {
                let event = new Event("click", { bubbles: true });
                this._element.firstChild.dispatchEvent(event);
            }
        });
    }
        
    load(chats) {
        for (let i = 0; i < chats.length; i++) {
            this.add(new Chat(chats[i]));
        }
    }

    add(chat) {
        if (this.#chats.get(chat.id())) {
            return;
        }
        if (!(chat instanceof Chat)) {
           chat = new Chat(chat);
        }
        this.#chats.set(chat.id(), chat);
        this._element.innerHTML += this.newElement(chat);
    }

    get(id) {
        return this.#chats.get(id);
    }

    refreshChatView(chat) {
        const item = this.getChatElement(chat.id());
        if (item) {
            item.innerHTML = this.elementView(chat);
        }
    }

    upElement(chat) {
        const item = this.getChatElement(chat.id());
        if (item) {
            item.innerHTML = this.elementView(chat);
            const outerHTML = item.outerHTML;
            this._element.removeChild(item);
            this._element.innerHTML = outerHTML + this._element.innerHTML;
        }
    }
    
    getChatElement(id) {
        for (let item = this._element.firstChild; item; item = item.nextSibling) {
            const cid = item.getAttribute(Chat.metaProperty);
            if (cid && Number(cid) == id) {
                return item;
            }
        }
    }

    markChatAsCurrent(chat) {
        let item = this.getChatElement(chat.id()); 
        if (item && !item.classList.contains('dialog-chatlist-item-current')) {
            item.classList.add('dialog-chatlist-item-current');
        }
    }

    unmarkChatAsCurrent(chat) {
        let item = this.getChatElement(chat.id());
        if (item) {
            item.classList.remove('dialog-chatlist-item-current');
        }
    }

    show() {
        this._element.classList.remove('invisible');
    }

    hide() {
        if (!this._element.classList.contains('invisible')) {
            this._element.classList.add('invisible');
        }
    }
}

class Chat {
    #name;
    #presentation;
    #phone;
    #status;
    #lastMessageText;
    #lastMessageDate;
    #contacts;
    #savedInputText;

    constructor(data) {
        this._id = data.ID || 0;
        this._type = data.Type || 0;
        this.#name = data.Name || '';
        this.#contacts = data.Contacts || [];

        this.#presentation = data.Presentation || '';
        this.#phone = data.Phone || '';
        this.#status = data.Status || 0;
        this.#lastMessageText = data.LastMessageText || '';
        this.#lastMessageDate = (data.LastMessageDate) ? new Date(Date.parse(data.LastMessageDate)) : new Date(1, 1, 1);
        this._history = new History(this._id);

        this.#savedInputText = '';
    }

    static metaProperty = 'data-chat-id';

    static async create(data) {
        const chat = await fetchPostJSON('chat/create', data.toJSON()).then((chat) => {
            return new Chat(chat);
        });     
        return chat;
    }

    id() {
        return this._id;
    }

    presentation() {
        return this.#presentation;
    }

    setPresentation(presentation) {
        this.#presentation = presentation;
    }

    phone() {
        return this.#phone;
    }

    setPhone(phone) {
        this.#phone = phone;
    }

    status() {
        return this.#status;
    }

    setStatus(status) {
        this.#status = status;
    }

    lastMessageDate() {
        return this.#lastMessageDate;
    }

    lastMessageText() {
        return this.#lastMessageText;
    }

    sendMessage(ws, text, service = false) {
        const message = new Message({ ChatID: this.id(), Text: text, Service: service });
        ws.send(JSON.stringify(message.toJSON()));
    }

    addMessage(message) {
        if (!(message instanceof Message)) {
            message = new Message(message);
        }
        this._history.add(message);
        if (this.#lastMessageDate <= message.date()) {
            this.#lastMessageDate = message.date();
            this.#lastMessageText = message.text();
        }
        return message;
    }

    async messages() {
        return await this._history.messages();
    }

    async updateHistory() {
        return await this._history.update();
    }

    saveInputText(text) {
        this.#savedInputText = text;
    }

    getSavedInputText() {
        return this.#savedInputText;
    }

    toJSON() {
        return {
            ID: this._id,
            Type: this._type,
            Name: this.#name,
            Contacts: this.#contacts,

            Presentation:    this.#presentation,
            Status:          this.#status,
            Phone:           this.#phone,
            LastMessageText: this.#lastMessageText,
            LastMessageDate: this.#lastMessageDate
        };
    }
}

class History {
    #lastID;
    #messages;
    #firstUpdate;
    #gottenEndOfHistory;

    #load(history) {
        for (let i = 0; i < history.Messages.length ; i++) {
            this.add(history.Messages[i]);
        }
    }

    async #fetchSync() {
        if (!this._chatID) {
            return;
        }
        await fetchJSONSync(`chat/history?chat_id=${ this._chatID }&message_id=${ this.#lastID }`, (history) => {
            if (history.LastID) {
                this.#load(history);
            }
        })
    }

    constructor(chatID) {
        this._chatID = chatID;
        this.#lastID = 0;
        this.#messages = [];
        this.#firstUpdate = true;
        this.#gottenEndOfHistory = false;
    }

    async messages() {
        if (this.#firstUpdate && this._chatID) {
            this.#firstUpdate = !this.#firstUpdate;
            return await this.update();
        } 
        return this.#messages;   
    }

    add(message) {
        if (!(message instanceof Message)) { 
            message = new Message(message);
        }
        this.#messages.push(message);
        if (!this.#lastID || this.#lastID > message.id()) {
            this.#lastID = message.id();
        }
    }

    async update() {
        if (this.#gottenEndOfHistory) {
            return [];
        }
        let len = this.#messages.length;
        await this.#fetchSync();
        if (len == this.#messages.length) {
            this.#gottenEndOfHistory = true;    
        }
        return this.#messages.slice(len, this.#messages.length);
    }
}

class Message {
    #text;

    constructor(data) {
        this._chatID = data.ChatID;
        this._id = data.ID || 0;
        this._author = data.Author || '';
        this._authorID = data.AuthorID || 0;
        this.#text = data.Text || '';
        this._date = (data.Date) ? new Date(Date.parse(data.Date)) : new Date(1, 1, 1);
        this._service = data.Service || false;
    }

    id() {
        return this._id;
    }

    chatID() {
        return this._chatID;
    }

    text() {
        return this.#text;
    }

    date() {
        return this._date;
    }

    html() {
        let strData = this._date.toLocaleString('ru', {
            hour: 'numeric',
            minute: 'numeric'
        });
        let classMsg = '';
        if (this._authorID == user.id()) {
            classMsg = 'dialog-history-out-msg';
        } else {
            classMsg = 'dialog-history-in-msg';  
        }
        return `<div class="dialog-history-msg ${ classMsg }"><div>${this._author}</div>${this.#text}<div>${strData}</div></div>`;
    }

    toJSON() {
        return {
            ChatID: this._chatID,
            Text: this.#text,
            Service: this._service
        };
    }
}

class SearchChat extends Chat {
    constructor(vid, chat) {
        super(chat);
        this._vid = vid;
    }

    vid() {
        return this._vid ? this._vid : this.id();
    }
}

class SearchList {
    #chats;
  
    #find = throttle(function() {
            fetchJSON(`chat/search?phrase=${ this._input.value }`, (chats) => {
                this.#chats.clear();
                this._element.innerHTML = '';

                if (chats) {
                    this.load(chats);
                }

                const event = new Event('end', { bubbles: true });
                this._element.dispatchEvent(event);
            });    
        }, 500);

    constructor(onclick, onend) {
        this.#chats = new Map;
        this._element = document.getElementById('chat-search');
        this._element.addEventListener('click', onclick.bind(undefined, this));
        this._element.addEventListener('end', onend);
        this._input = document.getElementById('chat-search-phrase');
        this._input.addEventListener('input', this.#find.bind(this));
    }
        
    load(chats) {
        for (let i = 0; i < chats.length; i++) {
            this.add(chats[i]);
        }
    }
    
    add(chat) {
        if (!(chat instanceof SearchChat)) {
            const vid = chat.ID ? chat.ID : -this.#chats.size;
            chat = new SearchChat(vid, chat);
        }
        this.#chats.set(chat.vid(), chat);
        this._element.innerHTML += this.newElement(chat);
    }

    get(vid) {
        return this.#chats.get(vid);
    }

    newElement(chat) {
        return `<li class="dialog-chatlist-search-item dialog-common" ${Chat.metaProperty}="${chat.vid()}">${chat.presentation()}: <span>${chat.phone()}</span></li>`;
    }

    emptyInput() {
        return (this._input.value == '');
    }
    
    show() {
        this._element.classList.remove('invisible');
    }

    hide() {
        this._input.value = '';
        this.#chats.clear();

        if (!this._element.classList.contains('invisible')) {
            this._element.classList.add('invisible');
        }
        this._element.innerHTML = '';
    }
}

function throttle(callback, ms) {
    let id;
    return function() {
        const context = this;
        const args = arguments;

        clearTimeout(id);

        id = setTimeout(() => {
                    callback.apply(context, args);
                    clearTimeout(id);
                }, ms);
    }
}

function flushAndThrottle(callback, ms) {
    let id;
    return function() {
        const context = this;
        const args = arguments;
        if (id) {
            return;
        } else {
            callback.apply(context, args);
        }
        id = setTimeout(() => {
                    callback.apply(context, args);
                    id = undefined;
                }, ms);
    }
}

async function fetchJSONSync(request, callback) {
    const response = await fetch(`${reqURL}${request}`);
    const data = await response.json();
    callback(data);
}

function fetchJSON(request, callback) {
    fetch(`${reqURL}${request}`)
                    .then((response) => {
                            return response.json();
                        })
                    .then(callback);
}

async function fetchPostJSON(request, data = {}) {
    const response = await fetch(`${reqURL}${request}`, {
        method: "POST",
        cache: "no-cache",
        headers: {
            "Content-Type": "application/json",
        },
        body: JSON.stringify(data),
    });
    return await response.json();
 }

function sleep(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
}

function redirect() {
    window.location.replace(host);
}

const host = 'localhost:8000';
const reqURL = `http://${ host }/`;
const user = new User();