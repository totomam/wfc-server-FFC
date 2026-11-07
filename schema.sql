--
-- PostgreSQL database dump
--

-- Dumped from database version 14.9 (Ubuntu 14.9-0ubuntu0.22.04.1)
-- Dumped by pg_dump version 14.9 (Homebrew)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: users; Type: TABLE; Schema: public; Owner: wiilink
--

CREATE TABLE IF NOT EXISTS public.users (
    profile_id bigint NOT NULL,
    user_id bigint NOT NULL,
    gsbrcd character varying NOT NULL,
    password character varying NOT NULL,
    ng_device_id bigint,
    email character varying NOT NULL,
    unique_nick character varying NOT NULL,
    firstname character varying,
    lastname character varying DEFAULT ''::character varying
);


ALTER TABLE ONLY public.users
    ADD IF NOT EXISTS last_ip_address character varying DEFAULT ''::character varying,
    ADD IF NOT EXISTS last_ingamesn character varying DEFAULT ''::character varying,
    ADD IF NOT EXISTS has_ban boolean DEFAULT false,
    ADD IF NOT EXISTS ban_issued timestamp without time zone,
    ADD IF NOT EXISTS ban_expires timestamp without time zone,
    ADD IF NOT EXISTS ban_reason character varying,
    ADD IF NOT EXISTS ban_reason_hidden character varying,
    ADD IF NOT EXISTS ban_moderator character varying,
    ADD IF NOT EXISTS ban_tos boolean,
	ADD IF NOT EXISTS open_host boolean DEFAULT false;

--
-- Change ng_device_id from bigint to bigint[]
--
DO $$ 
BEGIN
    IF (SELECT data_type FROM information_schema.columns WHERE table_name='users' AND column_name='ng_device_id') != 'ARRAY' THEN
        ALTER TABLE public.users
            ALTER COLUMN ng_device_id TYPE bigint[] using array[ng_device_id];
    END IF;
END $$;

ALTER TABLE public.users OWNER TO wiilink;

--
-- Name: sake_records; Type: TABLE; Schema: public; Owner: wiilink
--

CREATE TABLE IF NOT EXISTS public.sake_records (
    game_id integer NOT NULL,
    table_id character varying NOT NULL,
    record_id integer NOT NULL DEFAULT (random() * 2147483647)::integer,
    owner_id integer NOT NULL,
    fields jsonb NOT NULL CHECK (jsonb_typeof(fields) = 'object' AND jsonb_array_length(jsonb_path_query_array(fields, '$.keyvalue().key')) <= 128),
    create_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    update_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT one_sake_record_constraint UNIQUE (game_id, table_id, record_id)
);

--
-- Name: sake_ratings; Type: TABLE; Schema: public; Owner: wiilink
-- One rating per (record, rating profile); the primary key makes rating atomic.
--

CREATE TABLE IF NOT EXISTS public.sake_ratings (
    game_id integer NOT NULL,
    table_id character varying NOT NULL,
    record_id integer NOT NULL,
    rater_profile_id integer NOT NULL,
    rating smallint NOT NULL,

    PRIMARY KEY (game_id, table_id, record_id, rater_profile_id),
    FOREIGN KEY (game_id, table_id, record_id) REFERENCES public.sake_records (game_id, table_id, record_id) ON DELETE CASCADE
);

--
-- Name: mario_kart_wii_sake; Type: TABLE; Schema: public; Owner: wiilink
--

CREATE TABLE IF NOT EXISTS public.mario_kart_wii_sake (
    regionid smallint NOT NULL CHECK (regionid >= 1 AND regionid <= 7),
    courseid smallint NOT NULL CHECK (courseid >= 0 AND courseid <= 32767),
    score integer NOT NULL CHECK (score > 0 AND score < 360000),
    pid integer NOT NULL CHECK (pid > 0),
    playerinfo varchar(108) NOT NULL CHECK (LENGTH(playerinfo) = 108),
    ghost bytea CHECK (ghost IS NULL OR (OCTET_LENGTH(ghost) BETWEEN 148 AND 10240)),

    CONSTRAINT one_time_per_course_constraint UNIQUE (courseid, pid)
);


ALTER TABLE ONLY public.mario_kart_wii_sake
    ADD IF NOT EXISTS id serial PRIMARY KEY,
    ADD IF NOT EXISTS upload_time timestamp without time zone;


ALTER TABLE public.mario_kart_wii_sake OWNER TO wiilink;

--
-- Name: gamestats_public_data; Type: TABLE; Schema: public; Owner: wiilink
--

CREATE TABLE IF NOT EXISTS public.gamestats_public_data (
    profile_id bigint NOT NULL,
    dindex character varying NOT NULL,
    ptype character varying NOT NULL,
    pdata character varying NOT NULL,
    modified_time timestamp without time zone NOT NULL,

    CONSTRAINT one_pdata_constraint UNIQUE (profile_id, dindex, ptype)
);

--
-- Name: users_profile_id_seq; Type: SEQUENCE; Schema: public; Owner: wiilink
--

CREATE SEQUENCE IF NOT EXISTS public.users_profile_id_seq
    AS integer
    START WITH 1000000000
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.users_profile_id_seq OWNER TO wiilink;

--
-- Name: users_profile_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: wiilink
--

ALTER SEQUENCE public.users_profile_id_seq OWNED BY public.users.profile_id;

--
-- Name: users profile_id; Type: DEFAULT; Schema: public; Owner: wiilink
--

ALTER TABLE ONLY public.users ALTER COLUMN profile_id SET DEFAULT nextval('public.users_profile_id_seq'::regclass);

--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: wiilink
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (profile_id);

--
-- Name: events; Type: TABLE; Schema: public; Owner: wiilink
--

CREATE TABLE IF NOT EXISTS public.events (
    id serial PRIMARY KEY,
    event_type character varying NOT NULL,
    event_data jsonb NOT NULL,
    event_time timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);

--
-- Name: match_results; Type: TABLE; Schema: public; Owner: wiilink
-- FFC PvP result reports (nas/dls_result.go, database/results.go): one row per match, both consoles report.
--

CREATE TABLE IF NOT EXISTS public.match_results (
    id bigserial PRIMARY KEY,
    pid_a bigint NOT NULL,
    pid_b bigint NOT NULL,
    winner_pid bigint,
    draw boolean NOT NULL DEFAULT false,
    time_up boolean NOT NULL DEFAULT false,
    modded boolean NOT NULL DEFAULT false,
    match_ver bigint NOT NULL DEFAULT 0,
    name_a character varying NOT NULL DEFAULT '',
    name_b character varying NOT NULL DEFAULT '',
    team_a jsonb NOT NULL,
    team_b jsonb NOT NULL,
    report_a bytea,
    report_b bytea,
    status character varying NOT NULL DEFAULT 'single',
    mismatch character varying NOT NULL DEFAULT '',
    queue character varying NOT NULL DEFAULT '',
    created_at timestamp without time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS match_results_pair_idx ON public.match_results (pid_a, pid_b, created_at);

ALTER TABLE public.match_results OWNER TO wiilink;

--
-- Name: ffc_plaza_events; Type: TABLE; Schema: public; Owner: wiilink
-- FFC event / boss Plaza team windows (database/plaza_events.go); a scheduled record is visible only inside a window.
--

CREATE TABLE IF NOT EXISTS public.ffc_plaza_events (
    id serial PRIMARY KEY,
    game_id integer NOT NULL,
    table_id character varying NOT NULL,
    record_id integer NOT NULL,
    name character varying NOT NULL DEFAULT '' CHECK (LENGTH(name) <= 64),
    start_at timestamp without time zone NOT NULL,
    end_at timestamp without time zone NOT NULL,
    created_at timestamp without time zone NOT NULL DEFAULT (now() AT TIME ZONE 'UTC'),
    CHECK (end_at > start_at),
    FOREIGN KEY (game_id, table_id, record_id) REFERENCES public.sake_records (game_id, table_id, record_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS ffc_plaza_events_record_idx ON public.ffc_plaza_events (game_id, table_id, record_id);

ALTER TABLE public.ffc_plaza_events OWNER TO wiilink;

--
-- Name: ffc_player_links; Type: TABLE; Schema: public; Owner: wiilink
-- FFC persistent player identity (database/player_links.go): alias profile -> canonical profile for the FFC stats.
--

CREATE TABLE IF NOT EXISTS public.ffc_player_links (
    alias_pid bigint PRIMARY KEY,
    canonical_pid bigint NOT NULL,
    note character varying NOT NULL DEFAULT '' CHECK (LENGTH(note) <= 64),
    created_at timestamp without time zone NOT NULL DEFAULT (now() AT TIME ZONE 'UTC'),
    CHECK (alias_pid <> canonical_pid)
);
CREATE INDEX IF NOT EXISTS ffc_player_links_canonical_idx ON public.ffc_player_links (canonical_pid);

ALTER TABLE public.ffc_player_links OWNER TO wiilink;

--
-- PostgreSQL database dump complete
--

