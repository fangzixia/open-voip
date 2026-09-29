package control

import (
	"context"

	"open-switch/internal/errs"
	"open-switch/internal/scope"
)

func (s *Service) guardExpectedVersion(ctx context.Context, callID string) error {
	exp := scope.ExpectedVersion(ctx)
	if exp <= 0 {
		return nil
	}
	view, err := s.GetCall(ctx, callID)
	if err != nil {
		return err
	}
	if view.Version != exp {
		return errs.VersionConflict(view, "通话版本已变化，请刷新后重试")
	}
	return nil
}
