package server

type Option func(*Archiver, *CheckpointStore)

func WithArchiver(archiver Archiver) Option {
	return func(current *Archiver, _ *CheckpointStore) {
		if archiver != nil {
			*current = archiver
		}
	}
}

func WithCheckpointStore(store CheckpointStore) Option {
	return func(_ *Archiver, current *CheckpointStore) {
		if store != nil {
			*current = store
		}
	}
}
