package backup

import (
	"errors"

	backupv1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/backup/v1"
	"go.abhg.dev/cardamom/internal/project"
)

func projectToProto(snapshot project.Snapshot) (*backupv1.Project, error) {
	createdAt, err := timestampToProto("project creation time", snapshot.Created)
	if err != nil {
		return nil, err
	}
	return &backupv1.Project{
		Id: snapshot.ID.String(), Name: snapshot.Name, CreatedAt: createdAt,
	}, nil
}

func projectFromProto(encoded *backupv1.Project) (project.Snapshot, error) {
	if encoded == nil {
		return project.Snapshot{}, errors.New("project is required")
	}
	id, err := project.NewID(encoded.GetId())
	if err != nil {
		return project.Snapshot{}, err
	}
	createdAt, err := timestampFromProto("project creation time", encoded.GetCreatedAt())
	if err != nil {
		return project.Snapshot{}, err
	}
	snapshot := project.Snapshot{ID: id, Name: encoded.GetName(), Created: createdAt}
	if _, err := project.Load(snapshot); err != nil {
		return project.Snapshot{}, err
	}
	return snapshot, nil
}
