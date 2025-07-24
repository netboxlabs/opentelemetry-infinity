package otlpinf

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	yson "github.com/ghodss/yaml"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"

	"github.com/netboxlabs/opentelemetry-infinity/config"
	"github.com/netboxlabs/opentelemetry-infinity/runner"
)

type returnPolicyData struct {
	State runner.State `yaml:"status"`
	config.Policy
}

type returnValue struct {
	Message string `json:"message"`
}

func (o *OltpInf) setupRouter() {
	gin.SetMode(gin.ReleaseMode)
	o.router = gin.New()

	// Routes
	o.router.GET("/api/v1/status", o.getStatus)
	o.router.GET("/api/v1/capabilities", o.getCapabilities)
	o.router.GET("/api/v1/policies", o.getPolicies)
	o.router.POST("/api/v1/policies", o.createPolicy)
	o.router.GET("/api/v1/policies/:policy", o.getPolicy)
	o.router.DELETE("/api/v1/policies/:policy", o.deletePolicy)
}

func (o *OltpInf) startServer() {
	o.setupRouter()
	serverHost := o.conf.ServerHost
	serverPort := strconv.FormatUint(o.conf.ServerPort, 10)
	go func() {
		serv := serverHost + ":" + serverPort
		o.logger.Info("starting otlp_inf server at: " + serv)
		if err := o.router.Run(serv); err != nil {
			o.logger.Error("shutting down the server", "error", err)
		}
	}()
}

func (o *OltpInf) getStatus(c *gin.Context) {
	o.stat.UpTime = time.Since(o.stat.StartTime)
	c.IndentedJSON(http.StatusOK, o.stat)
}

func (o *OltpInf) getCapabilities(c *gin.Context) {
	j, err := yson.YAMLToJSON(o.capabilities)
	if err != nil {
		c.IndentedJSON(http.StatusBadRequest, returnValue{err.Error()})
		return
	}
	var ret interface{}
	err = json.Unmarshal(j, &ret)
	if err != nil {
		c.IndentedJSON(http.StatusBadRequest, returnValue{err.Error()})
		return
	}
	c.IndentedJSON(http.StatusOK, ret)
}

func (o *OltpInf) getPolicies(c *gin.Context) {
	policies := make([]string, 0, len(o.policies))
	for k := range o.policies {
		policies = append(policies, k)
	}
	c.IndentedJSON(http.StatusOK, policies)
}

func (o *OltpInf) getPolicy(c *gin.Context) {
	policy := c.Param("policy")
	rInfo, ok := o.policies[policy]
	if ok {
		c.YAML(http.StatusOK, map[string]returnPolicyData{policy: {rInfo.Instance.GetStatus(), rInfo.Policy}})
	} else {
		c.IndentedJSON(http.StatusNotFound, returnValue{"policy not found"})
	}
}

func (o *OltpInf) createPolicy(c *gin.Context) {
	if t := c.Request.Header.Get("Content-type"); t != "application/x-yaml" {
		c.IndentedJSON(http.StatusBadRequest, returnValue{"invalid Content-Type. Only 'application/x-yaml' is supported"})
		return
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.IndentedJSON(http.StatusBadRequest, returnValue{err.Error()})
		return
	}
	var payload map[string]config.Policy
	if err = yaml.Unmarshal(body, &payload); err != nil {
		c.IndentedJSON(http.StatusBadRequest, returnValue{err.Error()})
		return
	}

	for policy := range payload {
		_, ok := o.policies[policy]
		if ok {
			c.IndentedJSON(http.StatusConflict, returnValue{"policy '" + policy + "' already exists"})
			return

		}
	}

	var newPolicies []string
	newPolicyData := make(map[string]returnPolicyData)
	for policy, data := range payload {
		r := runner.NewRunner(o.logger, policy, o.policiesDir, o.conf)
		if err := r.Configure(&data); err != nil {
			for _, p := range newPolicies {
				r, ok := o.policies[p]
				if ok {
					r.Instance.Stop(o.ctx)
					delete(o.policies, policy)
				}
			}
			c.IndentedJSON(http.StatusBadRequest, returnValue{err.Error()})
			return
		}
		runnerCtx := context.WithValue(o.ctx, routineKey, policy)
		if err := r.Start(context.WithCancel(runnerCtx)); err != nil {
			for _, p := range newPolicies {
				r, ok := o.policies[p]
				if ok {
					r.Instance.Stop(o.ctx)
					delete(o.policies, policy)
				}
			}
			c.IndentedJSON(http.StatusBadRequest, returnValue{err.Error()})
			return
		}
		o.policies[policy] = RunnerInfo{Policy: data, Instance: r}
		newPolicies = append(newPolicies, policy)
		newPolicyData[policy] = returnPolicyData{r.GetStatus(), data}
	}
	c.YAML(http.StatusCreated, newPolicyData)
}

func (o *OltpInf) deletePolicy(c *gin.Context) {
	policy := c.Param("policy")
	r, ok := o.policies[policy]
	if ok {
		r.Instance.Stop(o.ctx)
		delete(o.policies, policy)
		c.IndentedJSON(http.StatusOK, returnValue{policy + " was deleted"})
	} else {
		c.IndentedJSON(http.StatusNotFound, returnValue{"policy not found"})
	}
}
