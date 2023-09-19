window.addEventListener('load', async () => { 
    user = new User(await User.getData());  
    tokenRefresher.refresh(true);
    const chatManager = new ChatView(); 
});

class ChatView {
    #currentChat;
    #ws;
    #historyRestored;

    async #onMessage(event) {
        const rawMessage = JSON.parse(event.data);
        let chat = this._chatList.get(rawMessage.ChatID);
        if (rawMessage.Type == Message.messageTypes.Typing.id) {
            if (!chat || chat.id() != this.#currentChat.id() || user.id() == rawMessage.AuthorID) {
                return;
            } 
            this.#notify(rawMessage);
        } else {
            if (!chat) {
                await fetchJSONAsync(`chat/chat?chat_id=${ rawMessage.ChatID }`, (rawChat) => {
                    chat = this._chatList.add(rawChat);
                    if (!this.#currentChat) {
                        this.setCurrentChat(chat);
                    }
                })
            } else {
                const message = chat.addMessage(rawMessage);
                if (!message) {
                    return;
                }
                
                if (chat.id() == this.#currentChat.id()) {
                    this.#displayMessageInHistory(this._historyElement.innerHTML + message.html());
                }

                this._chatList.refreshChatView(chat);
            }
            this._chatList.upElement(chat);
        }
    }

    async #onSend(event, type) {
        let chat = this.#currentChat;
 
        if (!chat) {
            return;
        }

        let text = '';

        const isService = (type && type != Message.messageTypes.Ordinary);
        if (!isService) {
            text = this._inputElement.value;
            this._inputElement.value = '';
            type = Message.messageTypes.Ordinary;
        } else {
            text = type.text;
        }

        if (!text) {
            return;
        }

        if (!chat.id() && !isService) {
            chat = await Chat.create(chat);
            if (!chat) {
                return;
            }
            this._chatList.add(chat);
            this.setCurrentChat(chat);
        }

        if (this.#ws.readyState > 1) {
            await this.newWebSocketConn(); 
        }
        
        chat.sendMessage(this.#ws, text, type.id);
    }

    #onKeyDown(event) {
        if (event.key == "Enter") {
            event.preventDefault();
            this.#onSend(event);
        }
    }
    
    #onTypingText = flushAndThrottle(async (event) => {
        this.#onSend(event, Message.messageTypes.Typing);
    }, 1000, true);

    #notify = function() {
        let id;
        return function(rawMessage) {
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

    #onScrollHistory = throttle(async function(event) {
        const height = this._historyElement.scrollHeight;
        
        this.restoreHistory();

        while (event.target.scrollTop <= event.target.clientHeight*2) {
            const messages = await this.#currentChat.updateHistory();
            if (!messages.length) {
                break;
            }
            const html = this.#fillHistory(messages);
            this._historyElement.innerHTML = html + this._historyElement.innerHTML;
            event.target.scrollTop += this._historyElement.scrollHeight - height; 
        }
    }, 100);

    constructor() {
        //data
        this.#historyRestored = false;
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

        //action
        this.newWebSocketConn();
        this._chatList.update();
    }
    
    newWebSocketConn = tokenRefresher.wrapAsyncRequest(async function newWebSocketConn() {
        this.#ws = new WebSocket(`wss://${ host }/chat/ws`);
        this.#ws.addEventListener('message', this.#onMessage.bind(this)); 
        while (this.#ws.readyState < 1) {
            await sleep(100);
        }   
    });

    setCurrentChat(chat) {
        this.#historyRestored = false;
        this.#currentChat = chat;
        this._chatList.markChatAsCurrent(chat);
        this.refreshHeader();
        this._inputElement.value = chat.getSavedInputText();

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

    restoreHistory() {
        if (this.#historyRestored) {
            return;
        }
        const messages = this.#currentChat.messages();
        const html = this.#fillHistory(messages);
        this.#displayMessageInHistory(html);
        this.#historyRestored = true;
    }

    #fillHistory(messages) {
        let html = '';
        for (let i = messages.length-1; i >= 0; i--) {
            html = messages[i].html() + html;
        }
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

    static async getData() {
        return await fetchPostJSON('chat/user').then((data) => { return data });
    }

    constructor(data) {
        this._element = document.getElementById('chat-user-brief-info');
        this._id = data.ID || 0;
        this.#phone = data.Phone || '';
        this.#firstName = data.FirstName || '';
        this.#lastName = data.LastName || '';
        this.#fullName = data.FullName || '';

        this._element.innerHTML = `${ this.#fullName } ${ this.#phone }`;
    }

    id() {
        return this._id;
    }

    phone() {
        return this.#phone;
    }
        
    fullName() {
        return this.#fullName;
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
        return `<li class="dialog-chatlist-item dialog-commo" ${ Chat.metaProperty }="${ chat.id() }">${ this.elementView(chat) }</li>`;
    }

    elementView(chat) {
        let strData = chat.lastMessageDate().toLocaleString('ru', {
            month: 'short',
            day: 'numeric'
        });
        return `${ chat.presentation() }<span> ${ strData }</span><div>${ chat.lastMessageText().slice(0,30) }</div>`;  
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
        if (!(chat instanceof Chat)) {
           chat = new Chat(chat);
        }
        if (this.#chats.get(chat.id())) {
            return;
        }
        this.#chats.set(chat.id(), chat);
        this._element.innerHTML += this.newElement(chat);
        return chat;
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
            if (!chat) {
                return;
            }
            chat.Presentation = chat.Presentation || data.presentation();
            chat.Phone = chat.Phone || data.phone();
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

    async sendMessage(ws, text, type) {
        const message = new Message({ ChatID: this.id(), Text: text, Type: type});
        ws.send(JSON.stringify(message.toJSON()));
    }

    addMessage(message) {
        if (!(message instanceof Message)) {
            message = new Message(message);
        }
        if (this._history.add(message)) {
            if (this.#lastMessageDate <= message.date()) {
                this.#lastMessageDate = message.date();
                this.#lastMessageText = message.text();
            }
            return message;
        }
        return undefined;        
    }

    messages() {
        return this._history.messages();
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
    #firstID;
    #lastID;
    #messages;
    #gotEndOfHistory;

    #insert(message, i) {
        if (!(message instanceof Message)) { 
            message = new Message(message);
        }
        const context = this;
        const f = function(message, i) { context.#messages[i] = message };
        this.#modify(f, message, i);
        return true;
    }

    #modify(func, message, ...args) {
        func(message, ...args);

        if (!this.#firstID || this.#firstID > message.id()) {
            this.#firstID = message.id();
        }
        if (!this.#lastID || this.#lastID < message.id()) {
            this.#lastID = message.id();
        }
    }

    async #fetchAsync() {
        if (!this._chatID) {
            return;
        }
        await fetchJSONAsync(`chat/history?chat_id=${ this._chatID }&message_id=${ this.#firstID }`, (history) => {
            if (!history.LastID) {
                return;
            }
            const oldArr = this.#messages;
            this.#messages = new Array(oldArr.length + history.Messages.length);
            let j = 0;
            for (let i = history.Messages.length-1; i >= 0; i--) {
                if (this.#insert(history.Messages[i], j)) {
                    j++;
                }
            }
            for (let i = 0; i < oldArr.length; i++) {
                this.#messages[j] = oldArr[i];
                j++;
            }
            this.#messages = this.#messages.slice(0, j);
        })
    }

    constructor(chatID) {
        this._chatID = chatID;
        this.#firstID = 0;
        this.#lastID = 0;
        this.#messages = [];
        this.#gotEndOfHistory = false;
    }

    messages() {
        return this.#messages;   
    }

    add(message) {
        if (!(message instanceof Message)) { 
            message = new Message(message);
        }
        if (this.#lastID >= message.id()) {
            return false;
        }
        const context = this;
        const f = function(message) {
            context.#messages.push(message);
        };
        this.#modify(f, message);
        return true;
    }

    async update() {
        if (this.#gotEndOfHistory) {
            return [];
        }
        let len = this.#messages.length;
        await this.#fetchAsync();
        if (len == this.#messages.length) {
            this.#gotEndOfHistory = true;    
        }
        return this.#messages.slice(0, this.#messages.length - len);
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
        this._type = data.Type || Message.messageTypes.Ordinary.id;
    }

    static messageTypes = {
        'Ordinary': {id: 1, text: ''},
        'Typing': {id: 2, text: 'typing text...'}
    };

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
        return `<div class="dialog-history-msg ${ classMsg }"><div>${ this._author }</div>${ this.#text }<div>${ strData }</div></div>`;
    }

    toJSON() {
        return {
            ChatID: this._chatID,
            Text: this.#text,
            Type: this._type
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
        return `<li class="dialog-chatlist-search-item dialog-common" ${ Chat.metaProperty }="${ chat.vid() }">${ chat.presentation() }: <span>${ chat.phone() }</span></li>`;
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

class TokenRefresher {
    #pendingRequests;
    #refreshing;

    constructor() {
        this.#pendingRequests = 0;
        this.#refreshing = false;
    }

    wrapRequest(callback) {
        const context = this;
        return function() {
            while (context.#refreshing) {
                sleep(1000);
            }
            context.#pendingRequests ++;
            callback.apply(this, arguments);
            context.#pendingRequests --;
        }
    }

    wrapAsyncRequest(callback) {
        const context = this;
        return async function() {
            while (context.#refreshing) {
                sleep(1000);
            }
            context.#pendingRequests ++;
            const result = await callback.apply(this, arguments);
            context.#pendingRequests --;
            return result;
        }
    }

    async refresh(skip = false) {
        if (!skip) {
            const url = `${ reqURL }authentication/login/silent`;
            const urlTarget = `${ reqURL }authentication/finish/silent/ok`;
            const silentLoginFrame = window.frames["silent-login"];
            for (let attempt = 0; attempt < 3; attempt++) {
                this.#refreshing = true;
                while (this.#pendingRequests > 0) {
                    await sleep(300);
                }
                silentLoginFrame.location = url;
                while (silentLoginFrame.location == url) {
                    await sleep(300);
                }
                this.#refreshing = false; 
                
                if (silentLoginFrame.location == urlTarget) { 
                    break;    
                }

                if (attempt < 2) {
                   await sleep(600000); 
                }
            }
        }

        setTimeout(this.refresh.bind(this), 86400000);//one a day
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

function sleep(ms) {
    return new Promise(resolve => setTimeout(resolve, ms));
}

const tokenRefresher = new TokenRefresher();

const fetchJSONAsync = tokenRefresher.wrapAsyncRequest(async function(request, callback) {
    try {
        const response = await fetch(`${ reqURL }${ request }`);
        const data = await response.json();
        callback(data);
    } catch (error) {
        console.log(error);
    }    
});

const fetchJSON = tokenRefresher.wrapRequest(function(request, callback) {
    try {
        fetch(`${ reqURL }${ request }`)
                        .then((response) => {
                                return response.json();
                            })
                        .then(callback);
    } catch (error) {
        console.log(error);
    }
});

const fetchPostJSON = tokenRefresher.wrapAsyncRequest(async function(request, data = {}) {
    try {
        const response = await fetch(`${ reqURL }${ request }`, {
            method: "POST",
            cache: "no-cache",
            headers: {
                "Content-Type": "application/json",
            },
            body: JSON.stringify(data),
        });
        return await response.json();
    } catch (error) {
        console.log(error);
    }
 });

const host = 'localhost:8000';
const reqURL = `https://${ host }/`;

let user;