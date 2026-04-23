package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/bigduu/bodhi-server/api/dto"
	"github.com/bigduu/bodhi-server/internal/models"
)

type VersionHandler struct {
	db *sql.DB
}

func NewVersionHandler(db *sql.DB) *VersionHandler {
	return &VersionHandler{db: db}
}

func (h *VersionHandler) CheckUpdate(w http.ResponseWriter, r *http.Request) {
	platform := r.URL.Query().Get("platform")
	if platform == "" {
		platform = "all"
	}

	v, err := models.GetLatestVersion(r.Context(), h.db, platform)
	if err != nil {
		if err == models.ErrVersionNotFound {
			writeError(w, http.StatusNotFound, "no version available")
			return
		}
		writeInternalError(w, err, "failed to check update")
		return
	}

	writeJSON(w, http.StatusOK, dto.VersionResponse{
		ID:          v.ID,
		Version:     v.Version,
		Platform:    v.Platform,
		Changelog:   v.Changelog,
		DownloadURL: v.DownloadURL,
		IsLatest:    v.IsLatest,
		ForceUpdate: v.ForceUpdate,
		PublishedAt: v.PublishedAt,
		CreatedAt:   v.CreatedAt,
	})
}

func (h *VersionHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	versions, err := models.ListVersions(r.Context(), h.db)
	if err != nil {
		writeInternalError(w, err, "failed to list versions")
		return
	}

	resp := make([]dto.VersionResponse, 0, len(versions))
	for _, v := range versions {
		resp = append(resp, dto.VersionResponse{
			ID:          v.ID,
			Version:     v.Version,
			Platform:    v.Platform,
			Changelog:   v.Changelog,
			DownloadURL: v.DownloadURL,
			IsLatest:    v.IsLatest,
			ForceUpdate: v.ForceUpdate,
			PublishedAt: v.PublishedAt,
			CreatedAt:   v.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *VersionHandler) GetVersion(w http.ResponseWriter, r *http.Request, id string) {
	v, err := models.GetVersion(r.Context(), h.db, id)
	if err != nil {
		if err == models.ErrVersionNotFound {
			writeError(w, http.StatusNotFound, "version not found")
			return
		}
		writeInternalError(w, err, "failed to get version")
		return
	}

	writeJSON(w, http.StatusOK, dto.VersionResponse{
		ID:          v.ID,
		Version:     v.Version,
		Platform:    v.Platform,
		Changelog:   v.Changelog,
		DownloadURL: v.DownloadURL,
		IsLatest:    v.IsLatest,
		ForceUpdate: v.ForceUpdate,
		PublishedAt: v.PublishedAt,
		CreatedAt:   v.CreatedAt,
	})
}

func (h *VersionHandler) CreateVersion(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateVersionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Version == "" {
		writeError(w, http.StatusBadRequest, "version is required")
		return
	}
	if req.Platform == "" {
		req.Platform = "all"
	}

	v, err := models.CreateVersion(r.Context(), h.db, req.Version, req.Platform, req.Changelog, req.DownloadURL, req.ForceUpdate)
	if err != nil {
		if err == models.ErrVersionExists {
			writeError(w, http.StatusConflict, "version already exists")
			return
		}
		writeInternalError(w, err, "failed to create version")
		return
	}

	writeJSON(w, http.StatusCreated, dto.VersionResponse{
		ID:          v.ID,
		Version:     v.Version,
		Platform:    v.Platform,
		Changelog:   v.Changelog,
		DownloadURL: v.DownloadURL,
		IsLatest:    v.IsLatest,
		ForceUpdate: v.ForceUpdate,
		IsDraft:     v.IsDraft,
		PublishedAt: v.PublishedAt,
		CreatedAt:   v.CreatedAt,
	})
}

func (h *VersionHandler) UpdateVersion(w http.ResponseWriter, r *http.Request, id string) {
	var req dto.UpdateVersionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	v, err := models.UpdateVersion(r.Context(), h.db, id, req.Changelog, req.DownloadURL, req.Platform, req.ForceUpdate)
	if err != nil {
		if err == models.ErrVersionNotFound {
			writeError(w, http.StatusNotFound, "version not found")
			return
		}
		writeInternalError(w, err, "failed to update version")
		return
	}

	writeJSON(w, http.StatusOK, dto.VersionResponse{
		ID:          v.ID,
		Version:     v.Version,
		Platform:    v.Platform,
		Changelog:   v.Changelog,
		DownloadURL: v.DownloadURL,
		IsLatest:    v.IsLatest,
		ForceUpdate: v.ForceUpdate,
		PublishedAt: v.PublishedAt,
		CreatedAt:   v.CreatedAt,
	})
}

func (h *VersionHandler) PublishVersion(w http.ResponseWriter, r *http.Request, id string) {
	v, err := models.PublishVersion(r.Context(), h.db, id)
	if err != nil {
		if err == models.ErrVersionNotFound {
			writeError(w, http.StatusNotFound, "version not found")
			return
		}
		writeInternalError(w, err, "failed to publish version")
		return
	}

	writeJSON(w, http.StatusOK, dto.VersionResponse{
		ID:          v.ID,
		Version:     v.Version,
		Platform:    v.Platform,
		Changelog:   v.Changelog,
		DownloadURL: v.DownloadURL,
		IsLatest:    v.IsLatest,
		ForceUpdate: v.ForceUpdate,
		IsDraft:     v.IsDraft,
		PublishedAt: v.PublishedAt,
		CreatedAt:   v.CreatedAt,
	})
}

func (h *VersionHandler) UnpublishVersion(w http.ResponseWriter, r *http.Request, id string) {
	v, err := models.UnpublishVersion(r.Context(), h.db, id)
	if err != nil {
		if err == models.ErrVersionNotFound {
			writeError(w, http.StatusNotFound, "version not found")
			return
		}
		writeInternalError(w, err, "failed to unpublish version")
		return
	}

	writeJSON(w, http.StatusOK, dto.VersionResponse{
		ID:          v.ID,
		Version:     v.Version,
		Platform:    v.Platform,
		Changelog:   v.Changelog,
		DownloadURL: v.DownloadURL,
		IsLatest:    v.IsLatest,
		ForceUpdate: v.ForceUpdate,
		IsDraft:     v.IsDraft,
		PublishedAt: v.PublishedAt,
		CreatedAt:   v.CreatedAt,
	})
}

func (h *VersionHandler) DeleteVersion(w http.ResponseWriter, r *http.Request, id string) {
	if err := models.DeleteVersion(r.Context(), h.db, id); err != nil {
		if err == models.ErrVersionNotFound {
			writeError(w, http.StatusNotFound, "version not found")
			return
		}
		writeInternalError(w, err, "failed to delete version")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
