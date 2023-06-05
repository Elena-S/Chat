# Chat
Этот проект написан с целью закрепления и демонстрации навыков программирования на Go.

## Intro
Проект представляет из себя сервер для обмена сообщениями по протоколу websocket. Также реализовано REST API для вывода минимально необходимой информации для коммуникации. На данный момент сервер написан таким образом, что сообщения хранятся в СУБД PostgreSQL (с целью закрепления знаний и навыков работы с этой системой). По мимо этого, процесс аутентификации выполняется по технологии OAuth2 (в качестве сервера аутентификации используется Ory Hydra), а хранение паролей реализовано с использованием Vault от HashiCorp (также было настроено автораспечатывание).

## Installation
Для работы приложения требуется установка Docker и docker compose.
После их установки необходимо запустить следующие команды:
```
git clone https://github.com/Elena-S/Chat.git
cd ./Chat
docker compose up
```
В строке браузера указать адрес https://localhost:8000 и Let's chat!
