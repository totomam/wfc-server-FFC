package database

import "wwfc/logging"

func (c *Connection) UpdateTables() {
	_, _ = c.pool.Exec(c.ctx, `

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
		ADD IF NOT EXISTS open_host boolean DEFAULT false,
		ADD IF NOT EXISTS allow_default_keys boolean DEFAULT false,
		ADD IF NOT EXISTS last_ip_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP;
	`)

	_, _ = c.pool.Exec(c.ctx, `

	DO $$ 
	BEGIN
    	IF (SELECT data_type FROM information_schema.columns WHERE table_name='users' AND column_name='ng_device_id') != 'ARRAY' THEN
        	ALTER TABLE public.users
            	ALTER COLUMN ng_device_id TYPE bigint[] using array[ng_device_id];
    	END IF;
	END $$;

	`)

	_, _ = c.pool.Exec(c.ctx, `

	ALTER TABLE ONLY public.mario_kart_wii_sake
        ADD IF NOT EXISTS id serial PRIMARY KEY,
		ADD IF NOT EXISTS upload_time timestamp without time zone;
	
	`)

	// Sake records may hold up to 128 fields (Fossil Fighters: Champions Team Plaza stores ~85)
	_, err := c.pool.Exec(c.ctx, `
	DO $$
	DECLARE r record;
	BEGIN
		-- Only runs once: when the 128-field constraint is not there yet, replace the old (64-field) one
		IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'public.sake_records'::regclass AND conname = 'sake_records_fields_check') THEN
			FOR r IN SELECT conname FROM pg_constraint WHERE conrelid = 'public.sake_records'::regclass AND contype = 'c'
				AND pg_get_constraintdef(oid) LIKE '%jsonb_array_length%' LOOP
				EXECUTE 'ALTER TABLE public.sake_records DROP CONSTRAINT ' || quote_ident(r.conname);
			END LOOP;
			ALTER TABLE public.sake_records ADD CONSTRAINT sake_records_fields_check
				CHECK (jsonb_typeof(fields) = 'object' AND jsonb_array_length(jsonb_path_query_array(fields, '$.keyvalue().key')) <= 128);
		END IF;
	END $$;
	`)
	if err != nil {
		logging.Error("DATABASE", "Failed to update the sake_records field limit constraint:", err)
	}

	// Ratings live in their own table so clients can never read or forge them through record fields.
	_, err = c.pool.Exec(c.ctx, `
	CREATE TABLE IF NOT EXISTS public.sake_ratings (
		game_id integer NOT NULL,
		table_id character varying NOT NULL,
		record_id integer NOT NULL,
		rater_profile_id integer NOT NULL,
		rating smallint NOT NULL,

		PRIMARY KEY (game_id, table_id, record_id, rater_profile_id),
		FOREIGN KEY (game_id, table_id, record_id) REFERENCES public.sake_records (game_id, table_id, record_id) ON DELETE CASCADE
	);
	`)
	if err != nil {
		logging.Error("DATABASE", "Failed to create the sake_ratings table:", err)
	}

	// FFC event / boss Plaza team windows (plaza_events.go); must exist before GetSakeRecords runs.
	if _, err = c.pool.Exec(c.ctx, ffcPlazaEventsTable); err != nil {
		logging.Error("DATABASE", "Failed to create the ffc_plaza_events table:", err)
	}

	// FFC player identity links (player_links.go, ROADMAP #53).
	if err = c.CreatePlayerLinksTable(); err != nil {
		logging.Error("DATABASE", "Failed to create the ffc_player_links table:", err)
	}

	// FFC leaderboard names (lb_names.go, ROADMAP #52).
	if err = c.CreateLbNamesTable(); err != nil {
		logging.Error("DATABASE", "Failed to create the ffc_lb_names table:", err)
	}
}
