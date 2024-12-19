// skipcq: RVV-A0003 Allow os.Exit outside main() or init()
package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/krakendio/krakend-cobra/v2/dumper"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/luraproject/lura/v2/config"
	"github.com/luraproject/lura/v2/core"
	"github.com/luraproject/lura/v2/logging"
	"github.com/luraproject/lura/v2/proxy"
	krakendgin "github.com/luraproject/lura/v2/router/gin"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
)

var onlineSchema = "https://www.krakend.io/schema/v%s/krakend.json"

func errorMsg(content string) string {
	if !IsTTY {
		return content
	}
	return dumper.ColorRed + content + dumper.ColorReset
}

type LastSourcer interface {
	LastSource() ([]byte, error)
}

func NewCheckCmd(rawSchema string) Command {
	rawEmbedSchema = rawSchema
	return CheckCommand
}

func checkFunc(cmd *cobra.Command, _ []string) { // skipcq: GO-R1005
	if cfgFile == "" {
		cmd.Println(errorMsg("Please, provide the path to the configuration file with --config or see all the options with --help"))
		os.Exit(1)
		return
	}

	cmd.Printf("Parsing configuration file: %s\n", cfgFile)

	v, err := parser.Parse(cfgFile)
	if err != nil {
		cmd.Println(errorMsg("ERROR parsing the configuration file:") + fmt.Sprintf("\t%s\n", err.Error()))
		os.Exit(1)
		return
	}

	if schemaValidation {
		cmd.Print("Linting configuration file...\n")

		var data []byte
		var err error
		if ls, ok := parser.(LastSourcer); ok {
			data, err = ls.LastSource()
		} else {
			data, err = os.ReadFile(cfgFile)
		}

		if err != nil {
			cmd.Println(errorMsg("ERROR loading the configuration content:") + fmt.Sprintf("\t%s\n", err.Error()))
			os.Exit(1)
			return
		}

		var raw interface{}
		if err := json.Unmarshal(data, &raw); err != nil {
			cmd.Println(errorMsg("ERROR converting configuration content to JSON:") + fmt.Sprintf("\t%s\n", err.Error()))
			os.Exit(1)
			return
		}

		if len(schemaPath) > 0 && schemaFetchOnline {
			cmd.Println(errorMsg("You cannot use both the --schema and --online options simultaneously. These arguments are mutually exclusive."))
			os.Exit(1)
			return
		}

		// Falling back to latest schema if the --online flag is defined or the embed schema was not set
		if schemaFetchOnline || rawEmbedSchema == "" {
			schemaPath = fmt.Sprintf(onlineSchema, getVersionMinor(core.KrakendVersion))
		}

		var sch *jsonschema.Schema
		var compilationErr error
		if len(schemaPath) > 0 {
			cmd.Printf("Using schema %s\n", schemaPath)

			httpLoader := HTTPURLLoader(http.Client{
				Timeout: 10 * time.Second,
			})

			loader := jsonschema.SchemeURLLoader{
				"file":  jsonschema.FileLoader{},
				"http":  &httpLoader,
				"https": &httpLoader,
			}
			compiler := jsonschema.NewCompiler()
			compiler.UseLoader(loader)

			sch, compilationErr = compiler.Compile(schemaPath)
			if compilationErr != nil {
				cmd.Println(errorMsg("ERROR compiling the custom schema:") + fmt.Sprintf("\t%s\n", compilationErr.Error()))
				os.Exit(1)
				return
			}
		} else {
			rawSchema, parseError := jsonschema.UnmarshalJSON(strings.NewReader(rawEmbedSchema))
			if parseError != nil {
				cmd.Println(errorMsg("ERROR parsing the embed schema:") + fmt.Sprintf("\t%s\n", parseError.Error()))
				os.Exit(1)
				return
			}

			compiler := jsonschema.NewCompiler()
			compiler.AddResource("schema.json", rawSchema)

			sch, compilationErr = compiler.Compile("schema.json")
			if compilationErr != nil {
				cmd.Println(errorMsg("ERROR compiling the embed schema:") + fmt.Sprintf("\t%s\n", compilationErr.Error()))
				os.Exit(1)
				return
			}
		}

		if err = sch.Validate(raw); err != nil {
			cmd.Println(errorMsg("ERROR linting the configuration file:") + fmt.Sprintf("\t%s\n", err.Error()))
			os.Exit(1)
			return
		}
	}

	if debug > 0 {
		cc := dumper.NewWithColors(cmd, checkDumpPrefix, debug, IsTTY)
		if err := cc.Dump(v); err != nil {
			cmd.Println(errorMsg("ERROR checking the configuration file:") + fmt.Sprintf("\t%s\n", err.Error()))
			os.Exit(1)
			return
		}
	}

	if checkGinRoutes {
		if err := RunRouterFunc(v); err != nil {
			cmd.Println(errorMsg("ERROR testing the configuration file:") + fmt.Sprintf("\t%s\n", err.Error()))
			os.Exit(1)
			return
		}
	}

	if IsTTY {
		cmd.Printf("%sSyntax OK!%s\n", dumper.ColorGreen, dumper.ColorReset)
		return
	}
	cmd.Println("Syntax OK!")
}

var RunRouterFunc = func(cfg config.ServiceConfig) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New(r.(string))
		}
	}()

	gin.SetMode(gin.ReleaseMode)
	cfg.Debug = cfg.Debug || debug > 0
	if port != 0 {
		cfg.Port = port
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	krakendgin.DefaultFactory(proxy.DefaultFactory(logging.NoOp), logging.NoOp).NewWithContext(ctx).Run(cfg)
	cancel()
	return nil
}

func getVersionMinor(ver string) string {
	comps := strings.Split(ver, ".")
	if len(comps) < 2 {
		return ver
	}
	return fmt.Sprintf("%s.%s", comps[0], comps[1])
}

type HTTPURLLoader http.Client

func (l *HTTPURLLoader) Load(url string) (interface{}, error) {
	client := (*http.Client)(l)
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%s returned status code %d", url, resp.StatusCode)
	}
	defer resp.Body.Close()

	return jsonschema.UnmarshalJSON(resp.Body)
}
