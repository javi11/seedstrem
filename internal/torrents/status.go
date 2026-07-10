package torrents

import (
	"github.com/javib/seedstrem/internal/qbit"
	"github.com/javib/seedstrem/internal/store"
)

// Lifecycle status values seedstrem reports for a torrent. These mirror
// the RealDebrid vocabulary the admin UI already understands.
const (
	StatusMagnetConversion = "magnet_conversion"
	StatusWaitingFiles     = "waiting_files_selection"
	StatusQueued           = "queued"
	StatusDownloading      = "downloading"
	StatusDownloaded       = "downloaded"
	StatusError            = "error"
)

// DeriveStatus maps our lifecycle phase plus live qBittorrent state to a
// coarse status string.
//
//   - stickyError: a persisted local error (e.g. torrent vanished from qbit)
//   - filesKnown: qBittorrent has resolved metadata (file list non-empty)
//   - progress: qBittorrent progress over wanted files, 0..1
func DeriveStatus(phase, qbitState string, stickyError bool, filesKnown bool, progress float64) string {
	if stickyError {
		return StatusError
	}

	switch qbitState {
	case qbit.StateError, qbit.StateMissingFiles:
		return StatusError
	}

	if phase == store.PhaseAdded {
		if !filesKnown {
			return StatusMagnetConversion
		}
		return StatusWaitingFiles
	}

	// phase == selected
	if progress >= 1 {
		return StatusDownloaded
	}

	switch qbitState {
	case qbit.StateQueuedDL, qbit.StateCheckingDL, qbit.StateAllocating,
		qbit.StateStoppedDL, qbit.StatePausedDL, qbit.StateCheckingResumeData:
		return StatusQueued
	case qbit.StateDownloading, qbit.StateStalledDL, qbit.StateMetaDL, qbit.StateForcedDL:
		return StatusDownloading
	case qbit.StateUploading, qbit.StateStalledUP, qbit.StateStoppedUP, qbit.StatePausedUP,
		qbit.StateQueuedUP, qbit.StateForcedUP, qbit.StateCheckingUP, qbit.StateMoving:
		// Upload-side state but selected files not at 100%: the wanted
		// files finished (qbit reports progress over wanted files) —
		// treat as downloaded.
		return StatusDownloaded
	default:
		return StatusDownloading
	}
}
