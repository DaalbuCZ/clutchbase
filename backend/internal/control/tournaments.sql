CREATE TABLE teams (
    id text PRIMARY KEY,
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 48),
    captain_steam_id text NOT NULL REFERENCES players(steam_id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE team_members (
    team_id text NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    steam_id text NOT NULL UNIQUE REFERENCES players(steam_id) ON DELETE CASCADE,
    joined_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(team_id,steam_id)
);
CREATE TABLE team_invites (
    team_id text PRIMARY KEY REFERENCES teams(id) ON DELETE CASCADE,
    code_hash text NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL
);
CREATE TABLE tournaments (
    id text PRIMARY KEY,
    name text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
    players_per_side integer NOT NULL CHECK (players_per_side BETWEEN 1 AND 5),
    status text NOT NULL CHECK (status IN ('open','checkin','active','complete')),
    winner_team_id text,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE tournament_entries (
    tournament_id text NOT NULL REFERENCES tournaments(id) ON DELETE CASCADE,
    team_id text NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    status text NOT NULL CHECK (status IN ('registered','checked_in','eliminated','winner')),
    PRIMARY KEY(tournament_id,team_id)
);
CREATE TABLE tournament_matches (
    id text PRIMARY KEY,
    tournament_id text NOT NULL REFERENCES tournaments(id) ON DELETE CASCADE,
    round integer NOT NULL,
    number integer NOT NULL,
    team_a text REFERENCES teams(id),
    team_b text REFERENCES teams(id),
    winner_team_id text REFERENCES teams(id),
    status text NOT NULL CHECK (status IN ('pending','complete','bye')),
    UNIQUE(tournament_id,round,number)
);
CREATE INDEX tournament_entries_team ON tournament_entries(team_id);

CREATE OR REPLACE FUNCTION notify_clutchbase_tournament_state() RETURNS trigger AS $$
BEGIN
    PERFORM pg_notify('clutchbase_state', 'changed');
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
DO $$ DECLARE table_name text; BEGIN
    FOREACH table_name IN ARRAY ARRAY['teams','team_members','team_invites','tournaments','tournament_entries','tournament_matches'] LOOP
        EXECUTE format('CREATE TRIGGER %I AFTER INSERT OR UPDATE OR DELETE ON %I FOR EACH STATEMENT EXECUTE FUNCTION notify_clutchbase_tournament_state()', table_name || '_state_changed', table_name);
    END LOOP;
END $$;
