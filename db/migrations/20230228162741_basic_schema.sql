-- +goose Up

CREATE EXTENSION IF NOT EXISTS pg_trgm;

--/ CHAT_TYPES /----------------------------------------
CREATE SEQUENCE IF NOT EXISTS public.chat_types_id_seq;
    
CREATE TABLE IF NOT EXISTS public.chat_types
(
    id bigint NOT NULL DEFAULT nextval('chat_types_id_seq'::regclass),
    name character varying(100) COLLATE pg_catalog."default",
    CONSTRAINT chat_types_pkey PRIMARY KEY (id)
)
TABLESPACE pg_default;

ALTER SEQUENCE public.chat_types_id_seq
    OWNED BY chat_types.id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_types_id
    ON public.chat_types USING btree
    (id ASC NULLS LAST)
TABLESPACE pg_default;

--/ USERS /----------------------------------------
CREATE SEQUENCE IF NOT EXISTS public.users_id_seq;

CREATE TABLE IF NOT EXISTS public.users
(
    id bigint NOT NULL DEFAULT nextval('users_id_seq'::regclass),
    phone character varying(16) COLLATE pg_catalog."default",
    first_name character varying(100) COLLATE pg_catalog."default",
    last_name character varying(200) COLLATE pg_catalog."default",
    full_name character varying(300) COLLATE pg_catalog."default",
    search_name character varying(300) COLLATE pg_catalog."default",
    salt bytea,
    password_hash text COLLATE pg_catalog."default",
    CONSTRAINT users_pkey PRIMARY KEY (id)
)	
TABLESPACE pg_default;

ALTER SEQUENCE public.users_id_seq
    OWNED BY users.id;

CREATE INDEX IF NOT EXISTS idx_users_search_name
    ON public.users USING gin
    (search_name COLLATE pg_catalog."default" gin_trgm_ops)
TABLESPACE pg_default;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_id
    ON public.users USING btree
    (id ASC NULLS LAST)
TABLESPACE pg_default;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_phone
    ON public.users USING btree
    (phone COLLATE pg_catalog."default" ASC NULLS LAST)
TABLESPACE pg_default;

--/ CHATS /----------------------------------------
CREATE SEQUENCE IF NOT EXISTS public.chats_id_seq;

CREATE TABLE IF NOT EXISTS public.chats
(
    id bigint NOT NULL DEFAULT nextval('chats_id_seq'::regclass),
    name varchar(100) COLLATE pg_catalog."default",
    search_name character varying(100) COLLATE pg_catalog."default",
    type_id bigint,
    CONSTRAINT chats_pkey PRIMARY KEY (id),
    CONSTRAINT fk_chat_types_chats FOREIGN KEY (type_id)
        REFERENCES public.chat_types (id) MATCH SIMPLE
        ON UPDATE CASCADE
        ON DELETE RESTRICT
)
TABLESPACE pg_default;

ALTER SEQUENCE public.chats_id_seq
    OWNED BY chats.id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_chats_id_type_id
    ON public.chats USING btree
    (id ASC NULLS LAST, type_id ASC NULLS LAST)
TABLESPACE pg_default;

CREATE INDEX IF NOT EXISTS idx_chats_search_name
    ON public.chats USING gin
    (search_name COLLATE pg_catalog."default" gin_trgm_ops)
TABLESPACE pg_default;

--/ CHAT_CONTACTS /----------------------------------------      
CREATE TABLE IF NOT EXISTS public.chat_contacts
(
    chat_id bigint,
    user_id bigint,
    CONSTRAINT fk_chats_chat_contacts FOREIGN KEY (chat_id)
        REFERENCES public.chats (id) MATCH SIMPLE
        ON UPDATE CASCADE
        ON DELETE RESTRICT,
    CONSTRAINT fk_users_chat_contacts FOREIGN KEY (user_id)
        REFERENCES public.users (id) MATCH SIMPLE
        ON UPDATE CASCADE
        ON DELETE RESTRICT
)
TABLESPACE pg_default;

CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_contacts_chat_id_user_id
ON public.chat_contacts USING btree
(chat_id ASC NULLS LAST, user_id ASC NULLS LAST)
TABLESPACE pg_default;

--/ CHAT_MESSAGES /----------------------------------------
CREATE SEQUENCE IF NOT EXISTS public.chat_messages_id_seq;

CREATE TABLE IF NOT EXISTS public.chat_messages
(
    id bigint NOT NULL DEFAULT nextval('chat_messages_id_seq'::regclass),
    chat_id bigint,
    text text COLLATE pg_catalog."default",
    date timestamp with time zone,
    author_id bigint,
    CONSTRAINT chat_messages_pkey PRIMARY KEY (id),
    CONSTRAINT fk_chats_chat_messages FOREIGN KEY (chat_id)
        REFERENCES public.chats (id) MATCH SIMPLE
        ON UPDATE CASCADE
        ON DELETE RESTRICT,
    CONSTRAINT fk_users_chat_messages FOREIGN KEY (author_id)
        REFERENCES public.users (id) MATCH SIMPLE
        ON UPDATE CASCADE
        ON DELETE RESTRICT
)
TABLESPACE pg_default;

ALTER SEQUENCE public.chat_messages_id_seq
    OWNED BY chat_messages.id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_messages_id
    ON public.chat_messages USING btree
    (id ASC NULLS LAST)
TABLESPACE pg_default;

CREATE INDEX IF NOT EXISTS idx_chat_messages_chat_id_id
    ON public.chat_messages USING btree
    (chat_id ASC NULLS LAST, id ASC NULLS LAST)
TABLESPACE pg_default;    


-- +goose Down
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;
GRANT ALL ON SCHEMA public TO postgres, public;
