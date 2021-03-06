package controllers

import (
	"encoding/json"

	"fmt"

	"github.com/astaxie/beego"
	"github.com/astaxie/beego/logs"
	"github.com/ffan/tidb-operator/operator"
	"github.com/ffan/tidb-operator/pkg/util/mysqlutil"
)

var (
	// advertiseIP=beego.BConfig.Listen.HTTPSAddr
	statAPI = "http://%s:%d/tidb/api/v1/tidbs/%s"
)

// TidbController operations about tidb
type TidbController struct {
	beego.Controller
}

// Post create a tidb instance, and asynchronously install if the Phase is PhaseUndefined.
// @Title CreateTidb
// @Description create a tidb
// @Param	body	body	operator.Db	true	"body for tidb content"
// @Success 200
// @Failure 403 body is empty
// @router / [post]
func (dc *TidbController) Post() {
	b := dc.Ctx.Input.RequestBody
	if len(b) < 1 {
		dc.CustomAbort(403, "body is empty")
	}
	var (
		err error
		db  = operator.NewDb()
	)
	if err = db.Unmarshal(b); err != nil {
		dc.CustomAbort(400, fmt.Sprintf("parse body %v", err))
	}

	if !operator.NeedApproval(db.Owner.ID, uint(db.Tikv.Replicas), uint(db.Tidb.Replicas)) {
		db.Status.Phase = operator.PhaseUndefined
	}

	errHandler(
		dc.Controller,
		db.Save(),
		fmt.Sprintf("create tidb %s", db.Schema.Name),
	)
	dc.Data["json"] = db.GetName()
	dc.ServeJSON()
}

// Delete delete a tidb install
// @Title DeleteTidb
// @Description delete the tidb instance
// @Param	cell	path 	string	true "The cell you want to delete"
// @Success 200 {string} delete success!
// @Failure 403 cell is empty
// @router /:cell [delete]
func (dc *TidbController) Delete() {
	cell := dc.GetString(":cell")
	if len(cell) < 1 {
		dc.CustomAbort(403, "db name is nil")
	}
	errHandler(
		dc.Controller,
		operator.Delete(cell),
		"delete db "+cell,
	)
	dc.Data["json"] = 1
	dc.ServeJSON()
}

// Limit Check the user's request for resources
// @Title Limit
// @Description Whether the user creates tidb for approval
// @Param 	user 	path 	string 	true	"The user id"
// @Param	body	body 	operator.ApprovalConditions	true	"body for resource content"
// @Success 200
// @Failure 403 body is empty
// @router /:user/limit [post]
func (dc *TidbController) Limit() {
	user := dc.GetString(":user")
	if len(user) < 1 {
		dc.CustomAbort(403, "user id is nil")
	}
	ac := &operator.ApprovalConditions{}
	b := dc.Ctx.Input.RequestBody
	if len(b) < 1 {
		dc.CustomAbort(403, "body is empty")
	}
	if err := json.Unmarshal(b, ac); err != nil {
		dc.CustomAbort(400, fmt.Sprintf("Parse body error: %v", err))
	}
	limit := operator.NeedApproval(user, ac.KvReplicas, ac.DbReplicas)
	dc.Data["json"] = limit
	dc.ServeJSON()
}

// Patch tidb
// @Title Patch
// @Description partially update the specified Tidb
// @Param	cell	path	string	true	"The cell for pd name"
// @Param	body	body	operator.Db	true	"Data format reference: http://jsonpatch.com/"
// @Success 200
// @Failure 403 body is empty
// @Failure 403 unsupport operation
// @router /:cell [patch]
func (dc *TidbController) Patch() {
	cell := dc.GetString(":cell")
	b := dc.Ctx.Input.RequestBody
	if len(b) < 1 {
		dc.CustomAbort(403, "body is empty")
	}
	db, err := operator.GetDb(cell)
	errHandler(dc.Controller, err, fmt.Sprintf("get db %s", cell))

	newDb := db.Clone()
	if err = patch(b, newDb); err != nil {
		dc.CustomAbort(400, fmt.Sprintf("parse patch body err: %v", err))
	}
	errHandler(dc.Controller, db.Update(newDb), fmt.Sprintf("patch db %s", cell))
}

// Get a tidb
// @Title Get a tidb
// @Description get tidb by cell
// @Param	cell	path	string	true	"The cell for tidb name"
// @Success 200 {object} operator.Db
// @Failure 404 :key not found
// @router /:cell [get]
func (dc *TidbController) Get() {
	cell := dc.GetString(":cell")
	db, err := operator.GetDb(cell)
	errHandler(
		dc.Controller,
		err,
		fmt.Sprintf("get tidb %s", cell),
	)
	dc.Data["json"] = db
	dc.ServeJSON()
}

// @Title GetEvents
// @Description get all events
// @Param	cell	path	string	true	"The cell for tidb name"
// @Success 200 {object} operator.Events
// @router /:cell/events [get]
func (dc *TidbController) GetEvents() {
	cell := dc.GetString(":cell")
	if len(cell) < 1 {
		dc.CustomAbort(403, "cell is nil")
	}
	es, err := operator.GetEventsBy(cell)
	errHandler(dc.Controller, err, fmt.Sprintf("get %s events", cell))

	dc.Data["json"] = es
	dc.ServeJSON()
}

// @Title Migrate
// @Description migrate mysql data to tidb
// @Param 	cell 	path	string	true	"The database name for tidb"
// @Param	body	body	controllers.Migrator	true	"Body for src mysql"
// @Success 200
// @Failure 403 body is empty
// @router /:cell/migrate [post]
func (dc *TidbController) Migrate() {
	cell := dc.GetString(":cell")
	if len(cell) < 1 {
		dc.CustomAbort(403, "cell is nil")
	}
	b := dc.Ctx.Input.RequestBody
	if len(b) < 1 {
		dc.CustomAbort(403, "Body is empty")
	}
	m := &Migrator{
		Include: true,
		Notify:  true,
	}
	if err := json.Unmarshal(b, m); err != nil {
		dc.CustomAbort(400, fmt.Sprintf("Parse body error: %v", err))
	}
	db, err := operator.GetDb(cell)
	errHandler(dc.Controller, err, "get db "+cell)
	api := ""
	if m.Notify {
		api = fmt.Sprintf(statAPI, beego.BConfig.Listen.HTTPAddr, beego.BConfig.Listen.HTTPPort, cell)
	}
	errHandler(
		dc.Controller,
		db.Migrate(m.Mysql, api, m.Sync, m.Include, m.Tables),
		fmt.Sprintf("migrate mysql to tidb %s", cell),
	)
}

// Migrator a migrated target
type Migrator struct {
	mysqlutil.Mysql `json:",inline"`
	Include         bool     `json:"include"`
	Tables          []string `json:"tables,omitempty"`
	Sync            bool     `json:"sync,omitempty"`
	Notify          bool     `json:"notify"`
}

func errHandler(c beego.Controller, err error, msg string) {
	if err == nil {
		logs.Debug("controller:", msg)
		return
	}
	logs.Error("%s: %v", msg, err)
	c.CustomAbort(err2httpStatuscode(err), fmt.Sprintf("%s error: %v", msg, err))
}
