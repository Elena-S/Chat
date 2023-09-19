# Chat
Этот проект написан с целью закрепления и демонстрации навыков программирования на Go.

## Intro
Проект представляет из себя сервер для обмена сообщениями между пользователями по протоколу websocket. Для этого реализовано REST API, позволяющее выводить минимально необходимую информацию для коммуникации. На данный момент сервер написан таким образом, что почти вся информация хранится в СУБД PostgreSQL. Процесс аутентификации пользователей реализован с использованием протокола OAuth2 (в качестве сервера аутентификации используется Ory Hydra), для валидации state используется Redis, хранение паролей реализовано с использованием Vault от HashiCorp (также было настроено автораспечатывание), коммуникация между несколькими серверами реализована с использованием Kafka (есть возможность выбора между Redis Streams и Kafka, написан адаптер). Приложение написано с использованием пакета go.uber.org/fx (dependency injection).

## Installation
Для работы приложения требуется установка [Docker](https://docs.docker.com/engine/install/) и [Docker compose](https://docs.docker.com/compose/install/).
После их установки необходимо запустить следующие команды:
```
git clone -b start https://github.com/Elena-S/Chat.git
cd Chat
docker compose up
```
В строке браузера указать адрес https://localhost:8000 и Let's chat!
