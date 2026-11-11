package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"gitee.com/kobeyoung81/nanoadmin/internal/store"
)

// The prompt blocks of the session agents. The **shared head** is stored on its own
// (session_head_prompts, keyed without a mode, so build and plan cannot diverge);
// the **mode switch notes** still live on the agent types, one prompt each.

// promptBlocksResponse is the shape both planes read.
type promptBlocksResponse struct {
	Head      *store.SessionHeadPrompt `json:"head"`
	ModeNotes map[string]string        `json:"mode_notes"`
}

// modeNoteIDs are the session types whose prompt is a mode switch note.
var modeNoteIDs = []string{"plan", "build"}

func (s *Server) modeNotes() (map[string]string, error) {
	notes := make(map[string]string, len(modeNoteIDs))
	for _, id := range modeNoteIDs {
		t, err := s.catalog.GetAgentType(id)
		if err != nil {
			return nil, err
		}
		if t != nil {
			notes[id] = t.Prompt
		}
	}
	return notes, nil
}

// restGetPromptBlocks (admin) returns the platform prompt blocks.
func (s *Server) restGetPromptBlocks(w http.ResponseWriter, r *http.Request) {
	head, err := s.catalog.PlatformSessionHead()
	if err != nil {
		slog.Error("get session head prompt", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	notes, err := s.modeNotes()
	if err != nil {
		slog.Error("get mode notes", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, promptBlocksResponse{Head: head, ModeNotes: notes})
}

// restSavePromptBlock (admin) upserts one platform block:
//
//	{"block": "session-head", "content": "…"}
//
// The mode switch notes are edited through the agent type prompt they live on
// (PUT /api/v1/agent-types/{id}), so they are rejected here to keep one writer per
// value.
func (s *Server) restSavePromptBlock(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Block   string `json:"block"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if body.Block != "session-head" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "unknown prompt block: the mode switch notes are edited on the agent type",
		})
		return
	}
	if err := s.catalog.SaveSessionHead(store.PromptScopePlatform, "", body.Content); err != nil {
		slog.Error("save session head prompt", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"block": body.Block, "status": "saved"})
}
