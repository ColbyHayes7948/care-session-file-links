package appointments

import (
	"context"
	"errors"
	"fmt"
)

const linkLifetimeSeconds = 300

var ErrUnauthorized = errors.New("session is not allowed to access this patient file")

type FileAccess interface {
	VerifySession(ctx context.Context, sessionID string) (valid bool, userID string, err error)
	PresignDownload(ctx context.Context, bucket, key, requestID string, expiresSeconds int) (string, error)
}

type DownloadRequest struct {
	RequestID         string `json:"request_id"`
	SessionID         string `json:"session_id"`
	PatientID         string `json:"patient_id"`
	AppointmentID     string `json:"appointment_id"`
	FileKind          string `json:"file_kind"`
	AppointmentStatus string `json:"appointment_status"`
}

type DownloadResult struct {
	DownloadURL    string `json:"download_url"`
	ExpiresSeconds int    `json:"expires_seconds"`
	Notification   string `json:"notification"`
}

type Service struct {
	files  FileAccess
	bucket string
}

func NewService(files FileAccess, bucket string) *Service {
	return &Service{files: files, bucket: bucket}
}

func (s *Service) IssueLink(ctx context.Context, in DownloadRequest) (DownloadResult, error) {
	if err := validate(in); err != nil {
		return DownloadResult{}, err
	}
	valid, userID, err := s.files.VerifySession(ctx, in.SessionID)
	if err != nil {
		return DownloadResult{}, err
	}
	if !valid || userID != in.PatientID {
		return DownloadResult{}, ErrUnauthorized
	}

	key := fmt.Sprintf("patients/%s/appointments/%s/%s.pdf", in.PatientID, in.AppointmentID, in.FileKind)
	link, err := s.files.PresignDownload(ctx, s.bucket, key, in.RequestID, linkLifetimeSeconds)
	if err != nil {
		return DownloadResult{}, err
	}
	return DownloadResult{
		DownloadURL:    link,
		ExpiresSeconds: linkLifetimeSeconds,
		Notification:   appointmentNotification(in.AppointmentStatus),
	}, nil
}

func validate(in DownloadRequest) error {
	if in.RequestID == "" || in.SessionID == "" || in.PatientID == "" || in.AppointmentID == "" {
		return errors.New("request_id, session_id, patient_id, and appointment_id are required")
	}
	if in.FileKind != "visit-summary" && in.FileKind != "care-instructions" {
		return errors.New("file_kind must be visit-summary or care-instructions")
	}
	switch in.AppointmentStatus {
	case "scheduled", "rescheduled", "cancelled":
		return nil
	default:
		return errors.New("appointment_status must be scheduled, rescheduled, or cancelled")
	}
}

func appointmentNotification(status string) string {
	switch status {
	case "scheduled":
		return "Your appointment is scheduled. Open the care portal for details."
	case "rescheduled":
		return "Your appointment was updated. Open the care portal for the latest details."
	default:
		return "Your appointment was cancelled. Contact the care team if you need help."
	}
}
