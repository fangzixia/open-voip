package control

import (
	"context"

	"open-switch/internal/errs"
	"open-switch/internal/scope"
)

// runIdempotentMutation 在提供 Idempotency-Key 时将命令记入 os_commands 并支持成功重放。
func (s *Service) runIdempotentMutation(ctx context.Context, callID, typ, requestHash string, run func() error) error {
	key := scope.Idempotency(ctx)
	if key == "" || s.deps.Commands == nil {
		return run()
	}
	cmd, replay, err := s.deps.Commands.Accept(ctx, callID, key, requestHash, typ, map[string]any{"call_id": callID})
	if err != nil {
		return err
	}
	if replay {
		switch cmd.Status {
		case "succeeded":
			return nil
		case "failed":
			if cmd.ErrorCode != "" {
				return errs.Conflict(cmd.ErrorCode, cmd.ErrorCode)
			}
			return errs.Conflict("命令此前已失败", "")
		case "accepted", "running", "unknown":
			return errs.Conflict("命令仍在执行", "")
		}
	}
	if err := s.deps.Commands.MarkRunning(ctx, cmd.ID); err != nil {
		return err
	}
	runErr := run()
	if runErr != nil {
		code := ""
		if api := errs.AsAPIError(runErr); api != nil {
			code = api.Code
		}
		_ = s.deps.Commands.Complete(ctx, cmd.ID, "failed", code, map[string]any{"call_id": callID})
		return runErr
	}
	_ = s.deps.Commands.Complete(ctx, cmd.ID, "succeeded", "", map[string]any{"call_id": callID})
	return nil
}
