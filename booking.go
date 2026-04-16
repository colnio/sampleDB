package main

import (
	"context"
)

// Equipment represents a piece of lab equipment.
// This type is still used by admin.go and api.go while those packages are
// pending their own domain splits (Tasks 16–17).
type Equipment struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Location    string `json:"location"`
}

// getAllEquipment returns all equipment ordered by name.
// Used by admin.go and api.go until they are migrated to their own packages.
func getAllEquipment(ctx context.Context) ([]Equipment, error) {
	rows, err := theApp.DB.Query(ctx,
		`SELECT equipment_id, name, coalesce(description, 'N/A'), coalesce(location, 'N/A')
         FROM equipment
         ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var equipment []Equipment
	for rows.Next() {
		var eq Equipment
		err := rows.Scan(&eq.ID, &eq.Name, &eq.Description, &eq.Location)
		if err != nil {
			continue
		}
		equipment = append(equipment, eq)
	}
	return equipment, nil
}
