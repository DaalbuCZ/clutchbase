ALTER TABLE tournaments DROP CONSTRAINT tournaments_format_check;
ALTER TABLE tournaments ADD CONSTRAINT tournaments_format_check CHECK (format IN ('single_elimination','round_robin','double_elimination'));

ALTER TABLE tournament_matches DROP CONSTRAINT tournament_matches_status_check;
ALTER TABLE tournament_matches ADD CONSTRAINT tournament_matches_status_check CHECK (status IN ('pending','complete','bye','skipped'));

ALTER TABLE tournament_matches
    ADD COLUMN bracket_stage text NOT NULL DEFAULT 'single' CHECK (bracket_stage IN ('single','winners','losers','grand_final','reset_final')),
    ADD COLUMN winner_next_match_id text REFERENCES tournament_matches(id) DEFERRABLE INITIALLY DEFERRED,
    ADD COLUMN winner_next_slot smallint CHECK (winner_next_slot IN (1,2)),
    ADD COLUMN loser_next_match_id text REFERENCES tournament_matches(id) DEFERRABLE INITIALLY DEFERRED,
    ADD COLUMN loser_next_slot smallint CHECK (loser_next_slot IN (1,2)),
    ADD COLUMN source_a_match_id text REFERENCES tournament_matches(id) DEFERRABLE INITIALLY DEFERRED,
    ADD COLUMN source_b_match_id text REFERENCES tournament_matches(id) DEFERRABLE INITIALLY DEFERRED;
