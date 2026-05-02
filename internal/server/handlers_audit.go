package server

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/SEObserver/crawlobserver/internal/applog"
)

// AuditJob suit l'état d'un build d'audit Seeseo lancé via build_audit_auto.py
// (qui spawne lui-même fetch_haloscan + build_audit + export_pdf en chaîne).
type AuditJob struct {
	SessionID  string    `json:"session_id"`
	Status     string    `json:"status"` // queued / running / done / error
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	Logs       []string  `json:"logs"`
	OutputHTML string    `json:"output_html,omitempty"`
	OutputPDF  string    `json:"output_pdf,omitempty"`
	Error      string    `json:"error,omitempty"`
}

var (
	auditJobsMu sync.Mutex
	auditJobs   = map[string]*AuditJob{} // session_id → job (état en mémoire)
)

// resolveAuditDir retourne le chemin du repo seeseo-audit. Cherche d'abord la
// variable d'environnement SEESEO_AUDIT_DIR, puis ~/Documents/seeseo-audit/.
func resolveAuditDir() string {
	if env := os.Getenv("SEESEO_AUDIT_DIR"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	candidate := filepath.Join(home, "Documents", "seeseo-audit")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}

// handleBuildAudit lance le pipeline d'audit Seeseo en arrière-plan pour le
// crawl_session donné. Idempotent : si un job tourne déjà pour ce SID, on
// retourne son état courant sans re-spawn.
func (s *Server) handleBuildAudit(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	sessionID := r.PathValue("id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session id required")
		return
	}

	auditDir := resolveAuditDir()
	if auditDir == "" {
		writeError(w, http.StatusInternalServerError,
			"seeseo-audit repo introuvable (essayez SEESEO_AUDIT_DIR ou ~/Documents/seeseo-audit/)")
		return
	}
	scriptPath := filepath.Join(auditDir, "build_audit_auto.py")
	if _, err := os.Stat(scriptPath); err != nil {
		writeError(w, http.StatusInternalServerError,
			fmt.Sprintf("build_audit_auto.py introuvable dans %s", auditDir))
		return
	}

	auditJobsMu.Lock()
	if existing, ok := auditJobs[sessionID]; ok && existing.Status == "running" {
		auditJobsMu.Unlock()
		writeJSON(w, existing)
		return
	}
	job := &AuditJob{
		SessionID: sessionID,
		Status:    "running",
		StartedAt: time.Now(),
		Logs:      []string{},
	}
	auditJobs[sessionID] = job
	auditJobsMu.Unlock()

	go runAuditJob(job, auditDir, scriptPath)

	writeJSON(w, job)
}

// handleBuildAuditStatus retourne l'état actuel du job pour ce SID.
// Si aucun job n'est en mémoire mais qu'un livrable HTML existe déjà sur disque
// (ex. après un quit/relaunch de l'app), on retourne `done` avec les paths —
// l'utilisateur retrouve son audit prêt sans avoir à le relancer.
// Si rien : retourne {status: "idle"}.
func (s *Server) handleBuildAuditStatus(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session id required")
		return
	}
	auditJobsMu.Lock()
	job, ok := auditJobs[sessionID]
	auditJobsMu.Unlock()
	if ok {
		writeJSON(w, job)
		return
	}

	// Découverte sur disque : si l'audit a déjà été généré pour ce SID
	// (l'état mémoire est perdu après un restart de l'app, mais output/ persiste).
	if htmlPath, pdfPath := s.discoverAuditOnDisk(r.Context(), sessionID); htmlPath != "" {
		writeJSON(w, &AuditJob{
			SessionID:  sessionID,
			Status:     "done",
			OutputHTML: htmlPath,
			OutputPDF:  pdfPath,
		})
		return
	}

	writeJSON(w, map[string]string{"status": "idle"})
}

// discoverAuditOnDisk cherche un livrable déjà généré pour ce SID en se basant
// sur le nom du projet associé. Convention de nommage côté Python :
// output/audit-<slug>.html où <slug> = slugify(client_name) en snake_case.
// Le pipeline auto peut aussi suffixer `_auto` si un fichier projet manuel
// existait — on teste les deux variantes.
func (s *Server) discoverAuditOnDisk(ctx context.Context, sessionID string) (htmlPath, pdfPath string) {
	auditDir := resolveAuditDir()
	if auditDir == "" {
		return "", ""
	}
	if s.store == nil || s.keyStore == nil {
		return "", ""
	}
	sess, err := s.store.GetSession(ctx, sessionID)
	if err != nil || sess == nil || sess.ProjectID == nil || *sess.ProjectID == "" {
		return "", ""
	}
	proj, err := s.keyStore.GetProject(*sess.ProjectID)
	if err != nil || proj == nil {
		return "", ""
	}
	slug := slugifyForAudit(proj.Name)
	if slug == "" {
		return "", ""
	}
	for _, name := range []string{slug, slug + "_auto"} {
		htmlCandidate := filepath.Join(auditDir, "output", "audit-"+name+".html")
		if _, err := os.Stat(htmlCandidate); err == nil {
			pdfCandidate := filepath.Join(auditDir, "output", "audit-"+name+".pdf")
			if _, err := os.Stat(pdfCandidate); err == nil {
				return htmlCandidate, pdfCandidate
			}
			return htmlCandidate, ""
		}
	}
	return "", ""
}

// slugifyForAudit reproduit la logique de slugify() côté Python build_audit_auto.py.
// Lettres/chiffres/underscores uniquement, ne commence pas par un digit, lowercase.
// Ex: "seeseo.fr" → "seeseo_fr", "Bestheim" → "bestheim".
func slugifyForAudit(name string) string {
	var b strings.Builder
	last := byte(0)
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			last = byte(r)
		} else if last != '_' && b.Len() > 0 {
			b.WriteByte('_')
			last = '_'
		}
	}
	s := strings.Trim(b.String(), "_")
	if s == "" {
		return "client"
	}
	if s[0] >= '0' && s[0] <= '9' {
		s = "p_" + s
	}
	return s
}

// runAuditJob exécute python3 build_audit_auto.py --sid <SID> dans le dossier
// seeseo-audit, capture le stdout/stderr ligne par ligne, et met à jour le job
// avec les logs + le path du HTML généré.
func runAuditJob(job *AuditJob, auditDir, scriptPath string) {
	applog.Infof("audit", "build start session=%s dir=%s", job.SessionID, auditDir)

	// On résout python3 dans le PATH ; à défaut, fallback /usr/bin/python3 (macOS).
	pythonBin := "python3"
	if _, err := exec.LookPath(pythonBin); err != nil {
		pythonBin = "/usr/bin/python3"
	}

	cmd := exec.Command(pythonBin, scriptPath, "--sid", job.SessionID)
	cmd.Dir = auditDir
	// L'env hérite du process parent : ANTHROPIC_API_KEY, HALOSCAN_API_KEY, etc.
	// sont déjà résolus côté script Python via les configs SeeseoCrawler.

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		finishAuditJob(job, "error", fmt.Sprintf("stdout pipe: %v", err), "", "")
		return
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		finishAuditJob(job, "error", fmt.Sprintf("start: %v", err), "", "")
		applog.Errorf("audit", "start failed session=%s: %v", job.SessionID, err)
		return
	}

	// Streaming des logs ligne par ligne. Cap à 1000 lignes pour éviter la fuite.
	htmlPath := ""
	pdfPath := ""
	htmlRe := regexp.MustCompile(`Audit V\d+ généré\s*:\s*(\S+\.html)`)
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
		applog.Errorf("audit", "wait failed session=%s: %v", job.SessionID, err)
		finishAuditJob(job, "error", err.Error(), htmlPath, pdfPath)
		return
	}

	if htmlPath == "" {
		finishAuditJob(job, "error", "audit terminé sans HTML détecté dans la sortie", "", pdfPath)
		return
	}

	finishAuditJob(job, "done", "", htmlPath, pdfPath)
	applog.Infof("audit", "build done session=%s html=%s", job.SessionID, htmlPath)
}

func appendAuditLog(job *AuditJob, line string) {
	auditJobsMu.Lock()
	defer auditJobsMu.Unlock()
	job.Logs = append(job.Logs, line)
	const maxLogs = 1000
	if len(job.Logs) > maxLogs {
		job.Logs = job.Logs[len(job.Logs)-maxLogs:]
	}
}

func finishAuditJob(job *AuditJob, status, errMsg, htmlPath, pdfPath string) {
	auditJobsMu.Lock()
	defer auditJobsMu.Unlock()
	job.Status = status
	if errMsg != "" {
		job.Error = errMsg
	}
	if htmlPath != "" {
		job.OutputHTML = strings.TrimSpace(htmlPath)
	}
	if pdfPath != "" {
		job.OutputPDF = strings.TrimSpace(pdfPath)
	}
	job.FinishedAt = time.Now()
}

// resolveAuditOutputFile valide et retourne le path absolu d'un fichier output
// généré par l'audit. Sécurité : seul un nom de la forme `audit-*.html|.pdf` sous
// le dossier output/ est accepté (pas de path traversal possible).
func resolveAuditOutputFile(rel string) (string, *httpStatusError) {
	auditDir := resolveAuditDir()
	if auditDir == "" {
		return "", &httpStatusError{http.StatusNotFound, "seeseo-audit dir not found"}
	}
	if rel == "" {
		return "", &httpStatusError{http.StatusBadRequest, "missing file param"}
	}
	clean := filepath.Base(rel)
	if clean != rel || strings.Contains(clean, "..") {
		return "", &httpStatusError{http.StatusBadRequest, "invalid file param"}
	}
	if !(strings.HasPrefix(clean, "audit-") && (strings.HasSuffix(clean, ".html") || strings.HasSuffix(clean, ".pdf"))) {
		return "", &httpStatusError{http.StatusBadRequest, "only audit-*.html / audit-*.pdf allowed"}
	}
	full := filepath.Join(auditDir, "output", clean)
	if _, err := os.Stat(full); err != nil {
		return "", &httpStatusError{http.StatusNotFound, "not found"}
	}
	return full, nil
}

type httpStatusError struct {
	code int
	msg  string
}

// handleOpenAuditOutput sert le fichier HTML/PDF généré, en lecture seule.
func (s *Server) handleOpenAuditOutput(w http.ResponseWriter, r *http.Request) {
	full, herr := resolveAuditOutputFile(r.URL.Query().Get("file"))
	if herr != nil {
		http.Error(w, herr.msg, herr.code)
		return
	}
	http.ServeFile(w, r, full)
}

// handleOpenAuditInBrowser exécute `open <path>` (macOS) pour ouvrir le livrable
// dans le navigateur système (Chrome/Safari) plutôt que dans le webview embarqué
// de l'app desktop, qui ne supporte pas window.open(target="_blank") nativement.
func (s *Server) handleOpenAuditInBrowser(w http.ResponseWriter, r *http.Request) {
	if !requireFullAccess(w, r) {
		return
	}
	full, herr := resolveAuditOutputFile(r.URL.Query().Get("file"))
	if herr != nil {
		http.Error(w, herr.msg, herr.code)
		return
	}
	// macOS uniquement pour l'instant — le crawler tourne en .app sur Darwin.
	// Sur Linux on utiliserait `xdg-open`, sur Windows `start`.
	cmd := exec.Command("open", full)
	if err := cmd.Start(); err != nil {
		applog.Errorf("audit", "open file failed: %v", err)
		http.Error(w, fmt.Sprintf("open failed: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "opened", "file": filepath.Base(full)})
}
