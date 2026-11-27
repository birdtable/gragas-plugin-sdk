package declarative

import (
	"slices"

	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/model"
	"github.com/birdtable/gragas-plugin-sdk/plugin"
	"github.com/birdtable/gragas-plugin-sdk/wire"
)

// {config} and {output} are matched as whole words, as Findings matches {output}: the adapter reader refuses a placeholder inside a word
// (its refusal "an argument is a literal or a whole placeholder", held by the row TestAPlaceholderInsideAWordIsRefusedByTheAdapterReader),
// so one rule holds in both places.
//
// Plan is, for a config read of files, the rounds of rounds.go; and otherwise one run in round 0, final: the adapter's own argv, with its three placeholders put in the wire's words.
// {config} is {scratch}/<the first config's own path>, which the engine has already placed (nothing is placed here);
// {output} is {scratch}/out.<format>, collected at that path; {fixtures} is one word per fixture, {scratch}/<path>, the
// engine placing the fixtures. The program is the tool's name, the grant id. There are no read steps, and the level is
// the engine's default. For config the run is the adapter's config read of the target; for run, its run.
func (p *Plugin) Plan(req wire.PlanRequest) (wire.PlanResponse, error) {
	var argv []string
	var format string
	switch req.For {
	case wire.ForConfig:
		i := slices.IndexFunc(p.ad.Config, func(c model.ConfigRead) bool { return c.Target == model.TargetType(req.Target) })
		if i < 0 {
			return wire.PlanResponse{}, plugin.Unsupportedf("the adapter of %s reads no config for the target %s", p.ad.Tool, req.Target)
		}
		if p.ad.Config[i].Files != nil {
			return p.planFiles(p.ad.Config[i], req)
		}
		argv = p.ad.Config[i].Argv
	default:
		if !slices.Contains(p.ad.Targets, model.TargetType(req.Target)) {
			return wire.PlanResponse{}, plugin.Unsupportedf("the adapter of %s does not cover the target %s", p.ad.Tool, req.Target)
		}
		argv, format = p.ad.Run.Argv, string(p.ad.Run.Output.Format)
	}
	if req.Round != 0 {
		return wire.PlanResponse{}, plugin.Unsupportedf("the plan is one round, and round %d was asked", req.Round)
	}
	run := wire.Run{Program: p.ad.Tool, Argv: []string{}}
	for _, w := range argv {
		switch {
		case w == string(model.PlaceFixtures):
			for _, f := range req.Fixtures {
				run.Argv = append(run.Argv, "{scratch}/"+f)
			}
		case w == string(model.PlaceConfig):
			if len(req.Config) == 0 {
				// An empty config set is a fact about the project, not about the plugin: no steps, and that is all.
				return wire.PlanResponse{Steps: []wire.Step{}, Final: true}, nil
			}
			run.Argv = append(run.Argv, "{scratch}/"+req.Config[0].Path)
		case w == string(model.PlaceOutput):
			out := "out." + format
			run.Argv = append(run.Argv, "{scratch}/"+out)
			run.Collect = []string{out}
		default:
			run.Argv = append(run.Argv, w)
		}
	}
	return wire.PlanResponse{Steps: []wire.Step{{Run: &run}}, Final: true}, nil
}
