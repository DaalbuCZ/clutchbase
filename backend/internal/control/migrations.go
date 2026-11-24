package control

import (
	"context"
	_ "embed"
)

//go:embed schema.sql
var schema string

//go:embed machines.sql
var machineSchema string

//go:embed observability.sql
var observabilitySchema string

//go:embed access.sql
var accessSchema string

//go:embed removal.sql
var removalSchema string

//go:embed whitelist.sql
var whitelistSchema string

//go:embed live.sql
var liveSchema string

//go:embed players.sql
var playerSchema string

//go:embed presets.sql
var presetSchema string

//go:embed matches.sql
var matchSchema string

//go:embed match_sides.sql
var matchSidesSchema string

//go:embed live_mode.sql
var liveModeSchema string

//go:embed match_capacity.sql
var matchCapacitySchema string

//go:embed player_names.sql
var playerNamesSchema string

//go:embed match_load.sql
var matchLoadSchema string

//go:embed match_control.sql
var matchControlSchema string

//go:embed map_observation.sql
var mapObservationSchema string

//go:embed match_snapshot.sql
var matchSnapshotSchema string

//go:embed match_end.sql
var matchEndSchema string

//go:embed match_settings.sql
var matchSettingsSchema string

//go:embed match_expiry.sql
var matchExpirySchema string

//go:embed match_cancel.sql
var matchCancelSchema string

//go:embed reporter_update.sql
var reporterUpdateSchema string

//go:embed match_result.sql
var matchResultSchema string

//go:embed match_history.sql
var matchHistorySchema string

//go:embed machine_storage.sql
var machineStorageSchema string

//go:embed match_stats.sql
var matchStatsSchema string

//go:embed automatic_result.sql
var automaticResultSchema string

//go:embed automatic_player_stats.sql
var automaticPlayerStatsSchema string

//go:embed game_update.sql
var gameUpdateSchema string

//go:embed tournaments.sql
var tournamentSchema string

//go:embed tournament_rooms.sql
var tournamentRoomsSchema string

//go:embed tournament_veto.sql
var tournamentVetoSchema string

//go:embed match_overtime.sql
var matchOvertimeSchema string

//go:embed tournament_series.sql
var tournamentSeriesSchema string

//go:embed match_series_support.sql
var matchSeriesSupportSchema string

//go:embed game_build.sql
var gameBuildSchema string

//go:embed tournament_format.sql
var tournamentFormatSchema string

//go:embed tournament_double_elimination.sql
var tournamentDoubleEliminationSchema string

//go:embed team_management.sql
var teamManagementSchema string

//go:embed machine_resources.sql
var machineResourcesSchema string

//go:embed live_score.sql
var liveScoreSchema string

//go:embed tournament_cancel.sql
var tournamentCancelSchema string

//go:embed login_return.sql
var loginReturnSchema string

//go:embed tournament_schedule.sql
var tournamentScheduleSchema string

//go:embed restart.sql
var restartSchema string

//go:embed restart_capability.sql
var restartCapabilitySchema string

//go:embed reports.sql
var reportsSchema string

//go:embed match_votes.sql
var matchVotesSchema string

//go:embed game_update_cancel.sql
var gameUpdateCancelSchema string

//go:embed player_rooms.sql
var playerRoomsSchema string

//go:embed player_room_owner.sql
var playerRoomOwnerSchema string

//go:embed tournament_notifications.sql
var tournamentNotificationsSchema string

//go:embed series_map_results.sql
var seriesMapResultsSchema string

//go:embed access_requests.sql
var accessRequestsSchema string

//go:embed manual_ready.sql
var manualReadySchema string

//go:embed pause_votes.sql
var pauseVotesSchema string

//go:embed tournament_disputes.sql
var tournamentDisputesSchema string

//go:embed workshop.sql
var workshopSchema string

//go:embed tournament_result_revision.sql
var tournamentResultRevisionSchema string

//go:embed tournament_winner_history.sql
var tournamentWinnerHistorySchema string

// The first migration also adopts databases created before migration tracking.
func (a *API) Migrate(ctx context.Context) error {
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(438927104)"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (version integer PRIMARY KEY)"); err != nil {
		return err
	}
	for i, sql := range []string{schema, machineSchema, observabilitySchema, accessSchema, removalSchema, whitelistSchema, liveSchema, playerSchema, presetSchema, matchSchema, matchSidesSchema, liveModeSchema, matchCapacitySchema, playerNamesSchema, matchLoadSchema, matchControlSchema, mapObservationSchema, matchSnapshotSchema, matchEndSchema, matchSettingsSchema, matchExpirySchema, matchCancelSchema, reporterUpdateSchema, matchResultSchema, matchHistorySchema, machineStorageSchema, matchStatsSchema, automaticResultSchema, automaticPlayerStatsSchema, gameUpdateSchema, tournamentSchema, tournamentRoomsSchema, tournamentVetoSchema, matchOvertimeSchema, tournamentSeriesSchema, matchSeriesSupportSchema, gameBuildSchema, tournamentFormatSchema, teamManagementSchema, machineResourcesSchema, liveScoreSchema, tournamentCancelSchema, loginReturnSchema, tournamentScheduleSchema, restartSchema, restartCapabilitySchema, reportsSchema, matchVotesSchema, gameUpdateCancelSchema, playerRoomsSchema, playerRoomOwnerSchema, tournamentNotificationsSchema, seriesMapResultsSchema, accessRequestsSchema, manualReadySchema, pauseVotesSchema, tournamentDisputesSchema, workshopSchema, tournamentDoubleEliminationSchema, tournamentResultRevisionSchema, tournamentWinnerHistorySchema} {
		var applied bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)", i+1).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		if _, err = tx.Exec(ctx, sql); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO schema_migrations VALUES ($1)", i+1); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
