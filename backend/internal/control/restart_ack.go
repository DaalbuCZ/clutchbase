package control

import (
	"context"
	"errors"

	"clutchbase/internal/protocol"
	"github.com/jackc/pgx/v5"
)

// acknowledgeRestart must run after the host report has been recorded in the
// same transaction. Callers must serialize cancellation on the same host row.
func acknowledgeRestart(ctx context.Context, tx pgx.Tx, hostID, machineID string, capable bool, report protocol.Report) error {
	if !capable || !protocol.ValidCommandID(report.RestartID) {
		return nil
	}
	var desired, observed, requestID, phase string
	err := tx.QueryRow(ctx, `SELECT desired_state,observed_state FROM hosts WHERE id=$1 AND machine_id=$2 AND removed_at IS NULL FOR UPDATE`, hostID, machineID).Scan(&desired, &observed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	err = tx.QueryRow(ctx, `SELECT request_id,phase FROM server_restarts WHERE host_id=$1 FOR UPDATE`, hostID).Scan(&requestID, &phase)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if requestID != report.RestartID {
		return nil
	}
	switch {
	case phase == "stopping" && report.RestartPhase == "stopped" && desired == "stopped" && observed == "stopped":
		// Existing unclosed rooms or pending maintenance prohibit an automatic start.
		host, err := scan(tx.QueryRow(ctx, "SELECT "+columns+" FROM hosts WHERE id=$1", hostID))
		if err != nil {
			return err
		}
		var safe bool
		err = tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM match_rooms WHERE host_id=$1 AND (status='open' OR load_status='pending' OR cancel_status='pending')) AND NOT EXISTS(SELECT 1 FROM game_updates WHERE host_id=$1 AND status='pending') AND NOT EXISTS(SELECT 1 FROM hosts WHERE id=$1 AND (remove_requested OR reporter_command_status='pending' OR mode_command_status='pending' OR match_control_status='pending'))`, hostID).Scan(&safe)
		if err != nil {
			return err
		}
		if !safe || !host.Online || host.Error != "" || !host.WhitelistApplied || !host.PresetApplied {
			// The agent has a stopped fence; explicitly cancel it before allowing ordinary starts.
			_, err = tx.Exec(ctx, `UPDATE server_restarts SET phase='cancelling',updated_at=now() WHERE host_id=$1 AND request_id=$2`, hostID, requestID)
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE hosts SET desired_state='running',revision=revision+1 WHERE id=$1`, hostID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE server_restarts SET phase='starting',updated_at=now() WHERE host_id=$1 AND request_id=$2`, hostID, requestID)
	case phase == "starting" && report.RestartPhase == "complete" && desired == "running" && observed == "running":
		_, err = tx.Exec(ctx, `UPDATE server_restarts SET phase='complete',updated_at=now() WHERE host_id=$1 AND request_id=$2`, hostID, requestID)
	case phase == "cancelling" && report.RestartPhase == "cancelled" && desired == "stopped" && observed == "stopped":
		_, err = tx.Exec(ctx, `UPDATE server_restarts SET phase='cancelled',updated_at=now() WHERE host_id=$1 AND request_id=$2`, hostID, requestID)
	case (phase == "stopping" || phase == "starting") && report.RestartPhase == "failed":
		if _, err = tx.Exec(ctx, `UPDATE hosts SET desired_state='stopped',revision=revision+1 WHERE id=$1`, hostID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE server_restarts SET phase='failed',updated_at=now() WHERE host_id=$1 AND request_id=$2`, hostID, requestID)
	}
	return err
}
