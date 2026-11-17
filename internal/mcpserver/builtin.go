package mcpserver

import (
	"context"
	"errors"
	"fmt"

	"github.com/Sebas1705/WinForge/internal/mcpkit"
)

// Names of the tools this package defines itself.
const (
	ToolApplyPlan   = "winforge_apply_plan"
	ToolListPlans   = "winforge_list_plans"
	ToolDiscardPlan = "winforge_discard_plan"
)

type planIDArgs struct {
	PlanID string `json:"planId"`
}

var planIDSchema = mcpkit.Schema(`{"type":"object","properties":{"planId":{"type":"string","description":"The id returned by the tool that prepared the plan."}},"required":["planId"],"additionalProperties":false}`)

// Builtin returns the three generic plan tools. winforge_apply_plan is the only way anything is applied.
func Builtin(store *PlanStore) []mcpkit.Tool {
	return []mcpkit.Tool{
		{
			Name:  ToolListPlans,
			Title: "List pending plans",
			Description: "Lists the plans prepared in this session that are still waiting: id, kind, what each would do and when it expires. " +
				"Use it to show the person again what is waiting for their approval. Changes nothing.",
			InputSchema: mcpkit.NoArgs,
			Class:       mcpkit.Read,
			Handler: func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
				if err := mcpkit.Decode(c.Args, &struct{}{}); err != nil {
					return nil, err
				}
				return mcpkit.JSON(map[string]any{"plans": store.List()}), nil
			},
		},
		{
			Name:  ToolDiscardPlan,
			Title: "Discard a pending plan",
			Description: "Forgets a prepared plan so it can never be applied. Use it when the person declines or changes their mind. " +
				"Changes nothing on the PC.",
			InputSchema: planIDSchema,
			Class:       mcpkit.Prepare,
			Handler: func(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
				var a planIDArgs
				if err := mcpkit.Decode(c.Args, &a); err != nil {
					return nil, err
				}
				if !store.Discard(a.PlanID) {
					return mcpkit.Fail("%s", mcpkit.ErrPlanUnknown.Error()), nil
				}
				return mcpkit.Text("Plan %s discarded. It will not be applied.", a.PlanID), nil
			},
		},
		{
			Name:  ToolApplyPlan,
			Title: "Apply an approved plan",
			Description: "Runs a plan that a prepare tool stored in this session, once. Call it ONLY after the person has seen the plan's summary " +
				"and clearly approved it in this conversation. The plan id works once and expires after a few minutes; a plan that needs " +
				"administrator rights is refused unless this server runs elevated. It changes the PC.",
			InputSchema: planIDSchema,
			Class:       mcpkit.Act,
			Handler:     applyPlan,
		},
	}
}

func applyPlan(ctx context.Context, c *mcpkit.Call) (*mcpkit.Result, error) {
	var a planIDArgs
	if err := mcpkit.Decode(c.Args, &a); err != nil {
		return nil, err
	}
	p, err := c.Plans.Take(a.PlanID)
	if err != nil {
		if errors.Is(err, mcpkit.ErrPlanUnknown) {
			return mcpkit.Fail("%s Prepare the plan again and ask the person to approve the new one.", err.Error()), nil
		}
		return nil, err
	}
	if p.Admin && (c.Env == nil || c.Env.Elevated == nil || !c.Env.Elevated()) {
		return mcpkit.Fail("This plan needs administrator rights and the WinForge MCP server is not elevated. " +
			"Restart the assistant's MCP server elevated (run the assistant as administrator), then prepare the plan again. Nothing was changed."), nil
	}
	if p.Run == nil {
		return nil, fmt.Errorf("plan %q (%s) has nothing to run", a.PlanID, p.Kind)
	}
	res, err := p.Run(ctx, c)
	if err != nil {
		return nil, err
	}
	if res == nil {
		res = mcpkit.Text("Done.")
	}
	return res, nil
}
