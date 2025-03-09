package plugins

import (
	"fmt"
    	"os"

	"github.com/osteele/gojekyll/tags"
	"github.com/osteele/liquid"
	"github.com/osteele/liquid/render"
)

func init() {
	register("jekyll-env", jekyllEnvPlugin{})
}

type jekyllEnvPlugin struct{ plugin }

func (p jekyllEnvPlugin) ConfigureTemplateEngine(e *liquid.Engine) error {
	e.RegisterTag("env", envTag)
	return nil
}

func envTag(ctx render.Context) (string, error) {
	var (
		varname, varvalue string
	)
	argsline, err := ctx.ExpandTagArg()
	if err != nil {
		return "", err
	}
	args, err := tags.ParseArgs(argsline)
	if err != nil {
		return "", err
	}
	if len(args.Args) > 0 {
		varname = args.Args[0]
	}
	options, err := args.EvalOptions(ctx)
	if err != nil {
		return "", err
	}
	for name, value := range options {
		switch name {
		case "varname":
			varname = fmt.Sprint(value)
		default:
			return "", fmt.Errorf("unknown env argument: %s", name)
		}
	}
	if varname == "" {
		return "", fmt.Errorf("parse error in env tag parameters %s", argsline)
	}
    varvalue = os.Getenv(varname)
	return varvalue, err
}
