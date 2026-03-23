package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/momaek/tolato/internal/server/domain"
)

const defaultTimelineLimit = 50

type Service interface {
	CreateSession(ctx context.Context, title string) (string, error)
	DeleteSession(ctx context.Context, sessionID string) error
	ListSessions(ctx context.Context, clientID string) ([]SessionListItem, error)
	BuildSnapshot(ctx context.Context, clientID string, sessionID string) (Snapshot, error)
	ListRows(ctx context.Context, sessionID string, page domain.CursorPage) (TimelinePage, error)
	UpdateSubscriptions(ctx context.Context, clientID string, activeSessionID string, watchSessionIDs []string) error
}

type Repositories struct {
	Sessions      domain.SessionRepository
	Timelines     domain.TimelineRepository
	Tasks         domain.TaskRepository
	Executions    domain.ExecutionRepository
	Subscriptions SubscriptionRegistry
}

type SubscriptionRegistry interface {
	SetActive(clientID string, sessionID string)
	SetWatchSessions(clientID string, sessionIDs []string)
	ClearUnread(clientID string, sessionID string) (int, bool)
	UnreadCount(clientID string, sessionID string) int
}

type service struct {
	repos Repositories
	clock domain.Clock
	ids   domain.IDGenerator
}

type Option func(*service)

func WithClock(clock domain.Clock) Option {
	return func(s *service) {
		s.clock = clock
	}
}

func WithIDGenerator(ids domain.IDGenerator) Option {
	return func(s *service) {
		s.ids = ids
	}
}

func NewService(repos Repositories, options ...Option) Service {
	svc := &service{
		repos: repos,
		clock: systemClock{},
		ids:   randomIDGenerator{},
	}
	for _, option := range options {
		if option != nil {
			option(svc)
		}
	}
	return svc
}

type SessionListItem struct {
	SessionID           string               `json:"sessionId"`
	Title               string               `json:"title"`
	Status              domain.SessionStatus `json:"status"`
	UpdatedAt           string               `json:"updatedAt"`
	ActiveTargetSummary string               `json:"activeTargetSummary"`
	Unread              int                  `json:"unread"`
}

type Snapshot struct {
	Session             SnapshotSession            `json:"session"`
	HeaderState         HeaderState                `json:"headerState"`
	SidebarSummary      SidebarSummary             `json:"sidebarSummary"`
	ActiveTargetContext domain.ActiveTargetContext `json:"activeTargetContext"`
	PendingAction       *PendingAction             `json:"pendingAction,omitempty"`
	ComposerState       ComposerState              `json:"composerState"`
	Timeline            TimelinePage               `json:"timeline"`
	ExecutionState      *ExecutionState            `json:"executionState,omitempty"`
}

type SnapshotSession struct {
	ID                      string               `json:"id"`
	Title                   string               `json:"title"`
	Status                  domain.SessionStatus `json:"status"`
	CurrentOperationID      *string              `json:"currentOperationId,omitempty"`
	CurrentTaskID           *string              `json:"currentTaskId,omitempty"`
	CurrentExecutionGroupID *string              `json:"currentExecutionGroupId,omitempty"`
	UpdatedAt               string               `json:"updatedAt"`
	Revision                int64                `json:"revision"`
}

type HeaderState struct {
	Mode              string `json:"mode"`
	ActiveTargetLabel string `json:"activeTargetLabel"`
	ConnectionLabel   string `json:"connectionLabel"`
}

type SidebarSummary struct {
	SessionLabel string   `json:"sessionLabel"`
	LastUpdated  string   `json:"lastUpdated"`
	PrimaryText  string   `json:"primaryText"`
	Chips        []string `json:"chips"`
}

type PendingAction struct {
	Type   domain.PendingActionType `json:"type"`
	TaskID *string                  `json:"taskId,omitempty"`
}

type ComposerState struct {
	Disabled    bool   `json:"disabled"`
	Placeholder string `json:"placeholder"`
}

type TimelinePage struct {
	Rows             []domain.TimelineRow `json:"rows"`
	NextBeforeCursor *string              `json:"nextBeforeCursor,omitempty"`
	HasMoreBefore    bool                 `json:"hasMoreBefore"`
}

type ExecutionState struct {
	TaskID    string              `json:"taskId"`
	Status    domain.TaskStatus   `json:"status"`
	Aggregate *ExecutionAggregate `json:"aggregate,omitempty"`
	Summary   *string             `json:"summary,omitempty"`
}

type ExecutionAggregate struct {
	Total   int `json:"total"`
	Running int `json:"running"`
	Success int `json:"success"`
	Failed  int `json:"failed"`
}

func (s *service) CreateSession(ctx context.Context, title string) (string, error) {
	if s.repos.Sessions == nil {
		return "", errors.New("session repository is not configured")
	}

	now := s.clock.Now().UTC()
	sessions, err := s.repos.Sessions.List(ctx, domain.SessionFilter{})
	if err != nil {
		return "", err
	}

	title = strings.TrimSpace(title)
	if title == "" {
		title = defaultSessionTitle(len(sessions))
	}

	session := domain.Session{
		ID:        s.ids.NewID("sess"),
		Title:     title,
		Status:    domain.SessionStatusIdle,
		Revision:  1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.repos.Sessions.Create(ctx, session); err != nil {
		return "", err
	}
	return session.ID, nil
}

func (s *service) DeleteSession(ctx context.Context, sessionID string) error {
	if s.repos.Sessions == nil {
		return errors.New("session repository is not configured")
	}
	if strings.TrimSpace(sessionID) == "" {
		return domain.ErrInvalidArgument
	}

	session, err := s.repos.Sessions.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	if session.Status == domain.SessionStatusRunning || session.Status == domain.SessionStatusWaitingAsyncExecution {
		return domain.ErrSessionBusy
	}
	return s.repos.Sessions.Delete(ctx, sessionID)
}

func (s *service) ListSessions(ctx context.Context, clientID string) ([]SessionListItem, error) {
	sessions, err := s.repos.Sessions.List(ctx, domain.SessionFilter{})
	if err != nil {
		return nil, err
	}

	items := make([]SessionListItem, 0, len(sessions))
	for _, item := range sessions {
		items = append(items, SessionListItem{
			SessionID:           item.ID,
			Title:               item.Title,
			Status:              item.Status,
			UpdatedAt:           item.UpdatedAt.UTC().Format(timeLayout),
			ActiveTargetSummary: activeTargetSummary(item),
			Unread:              s.unreadCount(clientID, item.ID),
		})
	}

	return items, nil
}

func (s *service) BuildSnapshot(ctx context.Context, clientID string, sessionID string) (Snapshot, error) {
	if clientID != "" && s.repos.Subscriptions != nil {
		s.repos.Subscriptions.ClearUnread(clientID, sessionID)
	}

	session, err := s.repos.Sessions.Get(ctx, sessionID)
	if err != nil {
		return Snapshot{}, err
	}

	timeline, err := s.ListRows(ctx, sessionID, domain.CursorPage{Limit: defaultTimelineLimit})
	if err != nil {
		return Snapshot{}, err
	}

	snapshot := Snapshot{
		Session: SnapshotSession{
			ID:                      session.ID,
			Title:                   session.Title,
			Status:                  session.Status,
			CurrentOperationID:      session.CurrentOperationID,
			CurrentTaskID:           session.CurrentTaskID,
			CurrentExecutionGroupID: session.CurrentExecutionGroupID,
			UpdatedAt:               session.UpdatedAt.UTC().Format(timeLayout),
			Revision:                session.Revision,
		},
		HeaderState: HeaderState{
			Mode:              "ai_agent",
			ActiveTargetLabel: activeTargetLabel(session.ActiveTargetContext),
			ConnectionLabel:   "ws connected",
		},
		SidebarSummary: SidebarSummary{
			SessionLabel: fmt.Sprintf("Session · %s", session.Title),
			LastUpdated:  session.UpdatedAt.UTC().Format(timeLayout),
			PrimaryText:  sidebarPrimaryText(session),
			Chips:        sidebarChips(session),
		},
		ActiveTargetContext: session.ActiveTargetContext,
		PendingAction:       pendingActionView(session.PendingAction),
		ComposerState:       composerState(session.Status),
		Timeline:            timeline,
	}

	if session.CurrentTaskID != nil {
		executionState, execErr := s.buildExecutionState(ctx, *session.CurrentTaskID)
		if execErr != nil && execErr != domain.ErrNotFound {
			return Snapshot{}, execErr
		}
		snapshot.ExecutionState = executionState
	}

	return snapshot, nil
}

func (s *service) ListRows(ctx context.Context, sessionID string, page domain.CursorPage) (TimelinePage, error) {
	if page.Limit <= 0 {
		page.Limit = defaultTimelineLimit
	}

	allRows, err := s.repos.Timelines.ListBySession(ctx, sessionID, domain.CursorPage{})
	if err != nil {
		return TimelinePage{}, err
	}
	rows, err := s.repos.Timelines.ListBySession(ctx, sessionID, page)
	if err != nil {
		return TimelinePage{}, err
	}

	view := TimelinePage{
		Rows:          rows,
		HasMoreBefore: len(allRows) > len(rows),
	}
	if len(rows) > 0 && len(allRows) > len(rows) {
		oldestID := rows[0].ID
		view.NextBeforeCursor = &oldestID
	}

	return view, nil
}

func (s *service) UpdateSubscriptions(ctx context.Context, clientID string, activeSessionID string, watchSessionIDs []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.repos.Subscriptions == nil {
		return errors.New("subscription registry is not configured")
	}

	s.repos.Subscriptions.SetActive(clientID, activeSessionID)
	s.repos.Subscriptions.SetWatchSessions(clientID, watchSessionIDs)
	return nil
}

func (s *service) buildExecutionState(ctx context.Context, taskID string) (*ExecutionState, error) {
	task, err := s.repos.Tasks.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}

	aggregate, err := s.repos.Executions.AggregateByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}

	return &ExecutionState{
		TaskID:  task.ID,
		Status:  task.Status,
		Summary: task.Summary,
		Aggregate: &ExecutionAggregate{
			Total:   aggregate.Total,
			Running: aggregate.Running,
			Success: aggregate.Success,
			Failed:  aggregate.Failed + aggregate.Timeout + aggregate.Cancelled,
		},
	}, nil
}

func (s *service) unreadCount(clientID string, sessionID string) int {
	if clientID == "" || s.repos.Subscriptions == nil {
		return 0
	}
	return s.repos.Subscriptions.UnreadCount(clientID, sessionID)
}

func activeTargetSummary(session domain.Session) string {
	if session.ActiveTargetContext.DisplayLabel != "" {
		return session.ActiveTargetContext.DisplayLabel
	}
	if session.Title != "" {
		return session.Title
	}
	return "No target selected"
}

func activeTargetLabel(ctx domain.ActiveTargetContext) string {
	switch ctx.Status {
	case domain.TargetStatusConfirmed:
		return "Confirmed target: " + fallbackDisplayLabel(ctx.DisplayLabel)
	case domain.TargetStatusPendingConfirmation:
		return "Pending target confirmation: " + fallbackDisplayLabel(ctx.DisplayLabel)
	default:
		return "No target selected"
	}
}

func sidebarPrimaryText(session domain.Session) string {
	if session.ActiveTargetContext.DisplayLabel != "" {
		return session.ActiveTargetContext.DisplayLabel
	}
	if session.Title != "" {
		return session.Title
	}
	return "No active target"
}

func sidebarChips(session domain.Session) []string {
	chips := []string{string(session.Status)}
	switch session.ActiveTargetContext.Status {
	case domain.TargetStatusConfirmed:
		chips = append(chips, "confirmed")
	case domain.TargetStatusPendingConfirmation:
		chips = append(chips, "pending_target")
	}
	if session.PendingAction != nil {
		chips = append(chips, string(session.PendingAction.Type))
	}
	return chips
}

func defaultSessionTitle(existingCount int) string {
	if existingCount <= 0 {
		return "Console Session"
	}
	return fmt.Sprintf("Console Session %d", existingCount+1)
}

func pendingActionView(action *domain.PendingAction) *PendingAction {
	if action == nil {
		return nil
	}

	view := &PendingAction{Type: action.Type}
	var payload struct {
		TaskID *string `json:"taskId"`
	}
	if len(action.Payload) > 0 {
		_ = json.Unmarshal(action.Payload, &payload)
		view.TaskID = payload.TaskID
	}
	return view
}

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now().UTC()
}

type randomIDGenerator struct{}

func (randomIDGenerator) NewID(prefix string) string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}

	token := hex.EncodeToString(buf)
	if prefix == "" {
		return token
	}
	return strings.TrimSpace(prefix) + "_" + token
}

func composerState(status domain.SessionStatus) ComposerState {
	switch status {
	case domain.SessionStatusRunning:
		return ComposerState{Disabled: true, Placeholder: "Current operation is running"}
	case domain.SessionStatusPausedWaitTargetConfirmation:
		return ComposerState{Disabled: true, Placeholder: "Confirm target to continue current operation"}
	case domain.SessionStatusPausedWaitApproval:
		return ComposerState{Disabled: true, Placeholder: "Approve or reject to continue current operation"}
	case domain.SessionStatusWaitingAsyncExecution:
		return ComposerState{Disabled: true, Placeholder: "Execution in progress; waiting for results"}
	default:
		return ComposerState{
			Disabled:    false,
			Placeholder: "发送任务请求，AI 会先决定是否查询节点、确认目标、生成计划或进入审批",
		}
	}
}

func fallbackDisplayLabel(label string) string {
	if label != "" {
		return label
	}
	return "unknown target"
}

const timeLayout = "2006-01-02T15:04:05Z07:00"
