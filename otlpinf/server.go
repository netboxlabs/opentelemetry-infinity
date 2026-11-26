package otlpinf

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net"
	"net/http"
	"slices"
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

// listenAddr joins host and port for the listener, bracketing an IPv6 host.
// A host given already bracketed, as [::1], keeps working: JoinHostPort would
// bracket it again.
func listenAddr(host string, port uint64) string {
	if len(host) > 1 && host[0] == '[' && host[len(host)-1] == ']' {
		host = host[1 : len(host)-1]
	}
	return net.JoinHostPort(host, strconv.FormatUint(port, 10))
}

func (o *OltpInf) startServer() <-chan error {
	o.setupRouter()
	serverAddr := listenAddr(o.conf.ServerHost, o.conf.ServerPort)
	errCh := make(chan error, 1)

	o.httpServer = &http.Server{
		Addr:    serverAddr,
		Handler: o.router,
	}

	go func() {
		o.logger.Info("starting otlp_inf server", "address", serverAddr)
		if err := o.httpServer.ListenAndServe(); err != nil {
			if errors.Is(err, http.ErrServerClosed) {
				close(errCh)
				return
			}
			errCh <- err
		}
		close(errCh)
	}()

	return errCh
}

func (o *OltpInf) getStatus(c *gin.Context) {
	stat := o.stat
	stat.UpTime = time.Since(stat.StartTime)
	c.IndentedJSON(http.StatusOK, stat)
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
	c.IndentedJSON(http.StatusOK, o.policyNames())
}

func (o *OltpInf) getPolicy(c *gin.Context) {
	policy := c.Param("policy")
	rInfo, ok := o.lookupPolicy(policy)
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

	o.createMu.Lock()
	defer o.createMu.Unlock()

	// Name order makes a failure part-way through stop the same policies every time.
	names := slices.Sorted(maps.Keys(payload))
	for _, policy := range names {
		if _, ok := o.lookupPolicy(policy); ok {
			c.IndentedJSON(http.StatusConflict, returnValue{"policy '" + policy + "' already exists"})
			return
		}
	}

	// The request's policies are added together once all have started, so a
	// failure leaves none of them behind.
	started := make(map[string]RunnerInfo, len(names))
	stopStarted := func() {
		for _, info := range started {
			info.Instance.Stop(o.ctx)
		}
	}
	newPolicyData := make(map[string]returnPolicyData, len(names))
	for _, policy := range names {
		data := payload[policy]
		r := runner.NewRunner(o.logger, policy, o.policiesDir, o.conf)
		if err := r.Configure(&data); err != nil {
			stopStarted()
			c.IndentedJSON(http.StatusBadRequest, returnValue{err.Error()})
			return
		}
		runnerCtx := context.WithValue(o.ctx, routineKey, policy)
		if err := r.Start(context.WithCancel(runnerCtx)); err != nil {
			stopStarted()
			c.IndentedJSON(http.StatusBadRequest, returnValue{err.Error()})
			return
		}
		started[policy] = RunnerInfo{Policy: data, Instance: r}
		newPolicyData[policy] = returnPolicyData{r.GetStatus(), data}
	}
	o.addPolicies(started)
	c.YAML(http.StatusCreated, newPolicyData)
}

func (o *OltpInf) deletePolicy(c *gin.Context) {
	policy := c.Param("policy")
	r, ok := o.removePolicy(policy)
	if !ok {
		c.IndentedJSON(http.StatusNotFound, returnValue{"policy not found"})
		return
	}
	r.Instance.Stop(o.ctx)
	c.IndentedJSON(http.StatusOK, returnValue{policy + " was deleted"})
}
