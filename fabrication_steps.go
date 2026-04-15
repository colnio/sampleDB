package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// FabricationStep represents a single step in a sample's fabrication process.
type FabricationStep struct {
	ID             int
	SampleID       int
	StepOrder      int
	StepDate       *time.Time
	PersonUserID   *int
	PersonUsername *string
	Description    string
	EquipmentID    *int
	EquipmentName  *string
	CreatedAt      time.Time
}

// getFabricationSteps returns all steps for a sample, ordered by step_order.
func getFabricationSteps(sampleID int) ([]FabricationStep, error) {
	rows, err := dbPool.Query(context.Background(), `
		SELECT fs.step_id, fs.sample_id, fs.step_order, fs.step_date,
		       fs.person_user_id, u.username,
		       COALESCE(fs.description, ''),
		       fs.equipment_id, e.name,
		       fs.created_at
		  FROM fabrication_steps fs
		  LEFT JOIN users u ON fs.person_user_id = u.user_id
		  LEFT JOIN equipment e ON fs.equipment_id = e.equipment_id
		 WHERE fs.sample_id = $1
		 ORDER BY fs.step_order, fs.step_id`, sampleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var steps []FabricationStep
	for rows.Next() {
		var s FabricationStep
		if err := rows.Scan(
			&s.ID, &s.SampleID, &s.StepOrder, &s.StepDate,
			&s.PersonUserID, &s.PersonUsername,
			&s.Description,
			&s.EquipmentID, &s.EquipmentName,
			&s.CreatedAt,
		); err != nil {
			return nil, err
		}
		steps = append(steps, s)
	}
	return steps, rows.Err()
}

// addFabricationStep inserts a new step and returns the fully-joined record
// (including PersonUsername and EquipmentName from joined tables).
func addFabricationStep(sampleID int, step FabricationStep) (FabricationStep, error) {
	var id int
	err := dbPool.QueryRow(context.Background(), `
		INSERT INTO fabrication_steps
		            (sample_id, step_order, step_date, person_user_id, description, equipment_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING step_id`,
		sampleID, step.StepOrder, step.StepDate, step.PersonUserID,
		step.Description, step.EquipmentID,
	).Scan(&id)
	if err != nil {
		return FabricationStep{}, err
	}
	return getStepByID(id)
}

// updateFabricationStep updates an existing step by ID.
func updateFabricationStep(stepID int, step FabricationStep) error {
	tag, err := dbPool.Exec(context.Background(), `
		UPDATE fabrication_steps
		   SET step_order     = $1,
		       step_date      = $2,
		       person_user_id = $3,
		       description    = $4,
		       equipment_id   = $5
		 WHERE step_id = $6`,
		step.StepOrder, step.StepDate, step.PersonUserID,
		step.Description, step.EquipmentID, stepID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// deleteFabricationStep removes a step by ID.
func deleteFabricationStep(stepID int) error {
	tag, err := dbPool.Exec(context.Background(), `
		DELETE FROM fabrication_steps WHERE step_id = $1`, stepID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// getStepByID returns a single fabrication step with joined user and equipment names.
func getStepByID(stepID int) (FabricationStep, error) {
	var s FabricationStep
	err := dbPool.QueryRow(context.Background(), `
		SELECT fs.step_id, fs.sample_id, fs.step_order, fs.step_date,
		       fs.person_user_id, u.username,
		       COALESCE(fs.description, ''),
		       fs.equipment_id, e.name,
		       fs.created_at
		  FROM fabrication_steps fs
		  LEFT JOIN users u ON fs.person_user_id = u.user_id
		  LEFT JOIN equipment e ON fs.equipment_id = e.equipment_id
		 WHERE fs.step_id = $1`, stepID).Scan(
		&s.ID, &s.SampleID, &s.StepOrder, &s.StepDate,
		&s.PersonUserID, &s.PersonUsername,
		&s.Description,
		&s.EquipmentID, &s.EquipmentName,
		&s.CreatedAt,
	)
	if err != nil {
		return FabricationStep{}, err
	}
	return s, nil
}

// reorderFabricationSteps updates step_order for each step in orderedIDs (1-based).
// orderedIDs must contain exactly all step IDs belonging to sampleID.
func reorderFabricationSteps(sampleID int, orderedIDs []int) error {
	// Verify the supplied IDs cover all steps for this sample.
	var count int
	if err := dbPool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM fabrication_steps WHERE sample_id = $1",
		sampleID).Scan(&count); err != nil {
		return err
	}
	if count != len(orderedIDs) {
		return fmt.Errorf("reorder: got %d step IDs but sample has %d steps", len(orderedIDs), count)
	}

	tx, err := dbPool.Begin(context.Background())
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	for i, stepID := range orderedIDs {
		if _, err := tx.Exec(context.Background(),
			`UPDATE fabrication_steps SET step_order = $1 WHERE step_id = $2 AND sample_id = $3`,
			i+1, stepID, sampleID); err != nil {
			return err
		}
	}
	return tx.Commit(context.Background())
}
