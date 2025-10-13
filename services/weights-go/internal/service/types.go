package service

import "time"

// PublishInput is the internal representation of a publish request.
type PublishInput struct {
	RunID       string
	Step        int64
	Checksum    string
	ArtifactURI string
	InlineBytes []byte
	Metadata    map[string]string
	PublishedAt time.Time
}

// VersionSnapshot describes the currently promoted weights for a run.
type VersionSnapshot struct {
	RunID       string
	Step        int64
	Checksum    string
	ArtifactURI string
	InlineBytes []byte
	Metadata    map[string]string
	PublishedAt time.Time
}

// WatchOptions tune streaming semantics for subscriptions.
type WatchOptions struct {
	ReplayLatest bool
}
