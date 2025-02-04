package handlers

import (
	"encoding/json"
	"regexp"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type TableHandler struct {
	gptScript *gptscript.GPTScript
}

func NewTableHandler(gptScript *gptscript.GPTScript) *TableHandler {
	return &TableHandler{
		gptScript: gptScript,
	}
}

func (t *TableHandler) ListTables(req api.Context) error {
	var (
		assistantID = req.PathValue("assistant_id")
		result      = types.TableList{
			Items: []types.Table{},
		}
	)

	thread, err := getProjectThread(req, assistantID)
	if err != nil {
		return err
	}

	if thread.Status.WorkspaceID == "" {
		return req.Write(result)
	}

	return listTablesInWorkspace(req, t.gptScript, thread.Status.WorkspaceID)
}

var validTableName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func (t *TableHandler) GetRows(req api.Context) error {
	var (
		assistantID = req.PathValue("assistant_id")
		tableName   = req.PathValue("table_name")
		result      = types.TableRowList{
			Items: []types.TableRow{},
		}
	)

	if !validTableName.MatchString(tableName) {
		return types.NewErrBadRequest("invalid table name %s", tableName)
	}

	thread, err := getProjectThread(req, assistantID)
	if err != nil {
		return err
	}

	if thread.Status.WorkspaceID == "" {
		return req.Write(result)
	}

	return listTableRows(req, t.gptScript, thread.Status.WorkspaceID, tableName)
}

func listTablesInWorkspace(req api.Context, gClient *gptscript.GPTScript, workspaceID string) error {
	var (
		toolRef v1.ToolReference
		result  string
	)

	if err := req.Get(&toolRef, "database-ui"); err != nil {
		return err
	}
	run, err := gClient.Run(req.Context(), "list_database_tables from "+toolRef.Status.Reference, gptscript.Options{
		Workspace: workspaceID,
	})
	if err != nil {
		return err
	}

	defer run.Close()
	result, err = run.Text()
	if err != nil {
		return err
	}

	req.ResponseWriter.Header().Set("Content-Type", "application/json")
	_, err = req.ResponseWriter.Write([]byte(result))
	return err
}

func listTableRows(req api.Context, gptScript *gptscript.GPTScript, workspaceID, tableName string) error {
	var toolRef v1.ToolReference
	if err := req.Get(&toolRef, "database-ui"); err != nil {
		return err
	}
	input, err := json.Marshal(map[string]string{
		"table": tableName,
	})
	if err != nil {
		return err
	}
	run, err := gptScript.Run(req.Context(), "list_database_table_rows from "+toolRef.Status.Reference, gptscript.Options{
		Input:     string(input),
		Workspace: workspaceID,
	})
	if err != nil {
		return err
	}
	defer run.Close()
	result, err := run.Text()
	if err != nil {
		return err
	}

	req.ResponseWriter.Header().Set("Content-Type", "application/json")
	_, err = req.ResponseWriter.Write([]byte(result))
	return err
}
