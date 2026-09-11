package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"website.goodwin.vpn/plane/internal/agentclient"
	"website.goodwin.vpn/plane/internal/stack"
	"website.goodwin.vpn/plane/internal/store"
	"website.goodwin.vpn/plane/internal/sub"
)

func (s *Server) CollectTrafficLoop(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = time.Minute
	}
	t := time.NewTicker(every)
	defer t.Stop()
	s.CollectTraffic(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.CollectTraffic(context.Background())
		}
	}
}

func (s *Server) collectTrafficNow(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.CollectTraffic(r.Context()))
}

func (s *Server) CollectTraffic(ctx context.Context) map[string]any {
	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		log.Printf("traffic nodes: %v", err)
		return map[string]any{"ok": false, "error": err.Error()}
	}
	updated := 0
	added := int64(0)
	applied := 0
	groups := map[uuid.UUID]bool{}
	now := time.Now()
	for _, n := range nodes {
		if strings.TrimSpace(n.IPv4) == "" || strings.TrimSpace(n.AgentToken) == "" {
			continue
		}
		if !stack.HasFamily(n.Families, stack.FamilyVLESS) && !stack.HasFamily(n.AppliedFamilies, stack.FamilyVLESS) {
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, 12*time.Second)
		c := agentclient.New(fmt.Sprintf("http://%s:%d", n.IPv4, n.ControlPort), n.AgentToken)
		st, err := c.Stats(cctx)
		cancel()
		if err != nil {
			log.Printf("traffic %s: %v", n.Name, err)
			continue
		}
		for _, row := range st.Users {
			id, err := uuid.Parse(row.Email)
			if err != nil {
				continue
			}
			before, berr := s.store.User(ctx, id)
			was := berr == nil && sub.Entitled(before.Status, before.Expire, before.Upload, before.Download, before.Total, now)
			up, down, after, err := s.store.ApplyXrayTraffic(ctx, n.ID, id, row.Uplink, row.Downlink)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					continue
				}
				log.Printf("traffic user %s: %v", row.Email, err)
				continue
			}
			if up != 0 || down != 0 {
				updated++
				added += up + down
			}
			still := sub.Entitled(after.Status, after.Expire, after.Upload, after.Download, after.Total, now)
			if was && !still {
				groups[after.GroupID] = true
			}
		}
	}
	for gid := range groups {
		applied += s.applyGroupNodes(ctx, gid)
	}
	return map[string]any{"ok": true, "users_updated": updated, "bytes_added": added, "applied_nodes": applied}
}
