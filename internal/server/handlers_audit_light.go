package server

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/SEObserver/crawlobserver/internal/applog"
)

// Audit simple (version light) : même mécanique que l'audit complet, mais le
// sidecar est build_light_auto.py --sid <SID> --brand <seeseo|seo-paris>, qui
// déroule lui-même la chaîne (contexte + Haloscan, 10 pages YTG, audit complet,
// découpe light) et écrit « Audit light généré : <html> » puis « PDF généré : <pdf> ».
// Un job par crawl_session ; la marque est mémorisée dans le job.

var (
	auditLightJobs = map[string]*AuditJob{} // session_id → job light (état en mémoire)
)

var validLightBrands = map[string]bool{"seeseo": true, "seo-paris": true}

func (s *Server) handleBuildAuditLight(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	sessionID := r.PathValue("id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session id required")
		return
	}
	brand := r.URL.Query().Get("brand")
	if brand == "" {
		brand = "seeseo"
	}
	if !validLightBrands[brand] {
		writeError(w, http.StatusBadRequest, "brand must be seeseo or seo-paris")
		return
	}
	auditDir := resolveAuditDir()
	if auditDir == "" {
		writeError(w, http.StatusInternalServerError,
			"seeseo-audit repo introuvable (essayez SEESEO_AUDIT_DIR ou ~/Documents/seeseo-audit/)")
		return
	}
	scriptPath := filepath.Join(auditDir, "build_light_auto.py")
	if _, err := os.Stat(scriptPath); err != nil {
		writeError(w, http.StatusInternalServerError,
			fmt.Sprintf("build_light_auto.py introuvable dans %s", auditDir))
		return
	}

	auditJobsMu.Lock()
	if existing, ok := auditLightJobs[sessionID]; ok && existing.Status == "running" {
		auditJobsMu.Unlock()
		writeJSON(w, existing)
		return
	}
	job := &AuditJob{
		SessionID: sessionID,
		Brand:     brand,
		Status:    "running",
		StartedAt: time.Now(),
		Logs:      []string{},
	}
	auditLightJobs[sessionID] = job
	auditJobsMu.Unlock()

	go runAuditLightJob(job, auditDir, scriptPath, brand)

	writeJSON(w, job)
}

func (s *Server) handleBuildAuditLightStatus(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session id required")
		return
	}
	auditJobsMu.Lock()
	job, ok := auditLightJobs[sessionID]
	auditJobsMu.Unlock()
	if ok {
		writeJSON(w, job)
		return
	}
	// Découverte sur disque après un relaunch de l'app : output/audit-light-<slug>[-seo-paris].html
	if htmlPath, pdfPath, brand := s.discoverAuditLightOnDisk(r, sessionID); htmlPath != "" {
		writeJSON(w, &AuditJob{
			SessionID:  sessionID,
			Brand:      brand,
			Status:     "done",
			OutputHTML: htmlPath,
			OutputPDF:  pdfPath,
		})
		return
	}
	writeJSON(w, map[string]string{"status": "idle"})
}

func (s *Server) discoverAuditLightOnDisk(r *http.Request, sessionID string) (htmlPath, pdfPath, brand string) {
	auditDir := resolveAuditDir()
	if auditDir == "" || s.store == nil || s.keyStore == nil {
		return "", "", ""
	}
	sess, err := s.store.GetSession(r.Context(), sessionID)
	if err != nil || sess == nil || sess.ProjectID == nil || *sess.ProjectID == "" {
		return "", "", ""
	}
	proj, err := s.keyStore.GetProject(*sess.ProjectID)
	if err != nil || proj == nil {
		return "", "", ""
	}
	slug := slugifyForAudit(proj.Name)
	if slug == "" {
		return "", "", ""
	}
	for _, name := range []string{slug, slug + "_auto"} {
		for _, b := range []string{"seeseo", "seo-paris"} {
			suffix := ""
			if b != "seeseo" {
				suffix = "-" + b
			}
			htmlCandidate := filepath.Join(auditDir, "output", "audit-light-"+name+suffix+".html")
			if _, err := os.Stat(htmlCandidate); err == nil {
				pdfCandidate := filepath.Join(auditDir, "output", "audit-light-"+name+suffix+".pdf")
				if _, err := os.Stat(pdfCandidate); err != nil {
					pdfCandidate = ""
				}
				return htmlCandidate, pdfCandidate, b
			}
		}
	}
	return "", "", ""
}

// runAuditLightJob exécute python3 build_light_auto.py --sid <SID> --brand <brand>
// et suit ses logs. La dernière ligne « PDF généré » gagne (l'audit complet en
// imprime une avant la light).
func runAuditLightJob(job *AuditJob, auditDir, scriptPath, brand string) {
	applog.Infof("audit-light", "build start session=%s brand=%s dir=%s", job.SessionID, brand, auditDir)

	pythonBin := "python3"
	if _, err := exec.LookPath(pythonBin); err != nil {
		pythonBin = "/usr/bin/python3"
	}
	cmd := exec.Command(pythonBin, scriptPath, "--sid", job.SessionID, "--brand", brand)
	cmd.Dir = auditDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		finishAuditJob(job, "error", fmt.Sprintf("stdout pipe: %v", err), "", "")
		return
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		finishAuditJob(job, "error", fmt.Sprintf("start: %v", err), "", "")
		applog.Errorf("audit-light", "start failed session=%s: %v", job.SessionID, err)
		return
	}

	htmlPath, pdfPath := "", ""
	htmlRe := regexp.MustCompile(`Audit light généré\s*:\s*(\S+\.html)`)
	pdfRe := regexp.MustCompile(`PDF généré\s*:\s*(\S+\.pdf)`)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if m := htmlRe.FindStringSubmatch(line); m != nil {
			htmlPath = m[1]
		}
		if m := pdfRe.FindStringSubmatch(line); m != nil {
			pdfPath = m[1]
		}
		appendAuditLog(job, line)
	}
	if err := cmd.Wait(); err != nil {
		applog.Errorf("audit-light", "wait failed session=%s: %v", job.SessionID, err)
		finishAuditJob(job, "error", err.Error(), htmlPath, pdfPath)
		return
	}
	if htmlPath == "" {
		finishAuditJob(job, "error", "audit simple terminé sans HTML détecté dans la sortie", "", pdfPath)
		return
	}
	// Le PDF light porte le même nom que le HTML : on ignore un PDF de l'audit complet resté en mémoire.
	if pdfPath != "" && strings.TrimSuffix(filepath.Base(pdfPath), ".pdf") != strings.TrimSuffix(filepath.Base(htmlPath), ".html") {
		pdfPath = ""
	}
	finishAuditJob(job, "done", "", htmlPath, pdfPath)
	applog.Infof("audit-light", "build done session=%s html=%s", job.SessionID, htmlPath)
}
