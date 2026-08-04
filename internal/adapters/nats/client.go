package nats

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	natsgo "github.com/nats-io/nats.go"
	natsjs "github.com/nats-io/nats.go/jetstream"
	"github.com/newrelic/go-agent/v3/integrations/nrnats"
	"github.com/newrelic/go-agent/v3/newrelic"
)

type Client struct {
	connection *natsgo.Conn
	jetStream natsjs.JetStream
	monitoring *newrelic.Application
}

func New(ctx context.Context, url, name string, applications ...*newrelic.Application) (*Client, error) {
	if ctx == nil { return nil, fmt.Errorf("NATS context is required") }
	url=strings.TrimSpace(url);if url==""{return nil,fmt.Errorf("NATS URL is required")};name=strings.TrimSpace(name);if name==""{name="dugble"}
	var monitoring *newrelic.Application;if len(applications)>0{monitoring=applications[0]}
	connection,err:=natsgo.Connect(url,natsgo.Name(name),natsgo.Timeout(5*time.Second),natsgo.MaxReconnects(-1),natsgo.ReconnectWait(2*time.Second),natsgo.DisconnectErrHandler(func(_ *natsgo.Conn,disconnectErr error){if disconnectErr!=nil{slog.Warn("NATS disconnected","error",disconnectErr)}}),natsgo.ReconnectHandler(func(connection *natsgo.Conn){slog.Info("NATS reconnected","server",connection.ConnectedUrlRedacted())}),natsgo.ClosedHandler(func(connection *natsgo.Conn){if closeErr:=connection.LastError();closeErr!=nil{slog.Error("NATS connection closed","error",closeErr)}}))
	if err!=nil{return nil,fmt.Errorf("connect to NATS: %w",err)}
	jetStream,err:=natsjs.New(connection);if err!=nil{connection.Close();return nil,fmt.Errorf("initialize JetStream: %w",err)}
	if _,err:=jetStream.AccountInfo(ctx);err!=nil{connection.Close();return nil,fmt.Errorf("verify JetStream account: %w",err)}
	return &Client{connection:connection,jetStream:jetStream,monitoring:monitoring},nil
}

func(client *Client)Provision(ctx context.Context,limits StreamLimits)error{if client==nil||client.jetStream==nil{return fmt.Errorf("JetStream client is not configured")};for _,config:=range StreamConfigs(limits){if _,err:=client.jetStream.CreateOrUpdateStream(ctx,config);err!=nil{return fmt.Errorf("provision JetStream stream %s: %w",config.Name,err)}};return nil}
func(client *Client)CreateOrUpdateConsumer(ctx context.Context,stream string,config natsjs.ConsumerConfig)(natsjs.Consumer,error){if client==nil||client.jetStream==nil{return nil,fmt.Errorf("JetStream client is not configured")};stream=strings.TrimSpace(stream);if stream==""{return nil,fmt.Errorf("JetStream stream name is required")};consumer,err:=client.jetStream.CreateOrUpdateConsumer(ctx,stream,config);if err!=nil{return nil,fmt.Errorf("create or update consumer %s on %s: %w",config.Durable,stream,err)};return consumer,nil}
func(client *Client)Publish(ctx context.Context,subject string,payload []byte,headers map[string]string,messageID string)error{if client==nil||client.jetStream==nil||client.connection==nil{return fmt.Errorf("JetStream client is not configured")};message:=&natsgo.Msg{Subject:strings.TrimSpace(subject),Data:payload,Header:natsgo.Header{}};for key,value:=range headers{message.Header.Set(key,value)};txn:=newrelic.FromContext(ctx);ownsTransaction:=false;if txn==nil&&client.monitoring!=nil{txn=client.monitoring.StartTransaction("NATS publish "+message.Subject);ctx=newrelic.NewContext(ctx,txn);ownsTransaction=true};if ownsTransaction{defer txn.End()};if txn!=nil{txn.AddAttribute("messaging.system","nats");txn.AddAttribute("messaging.destination",message.Subject);txn.InsertDistributedTraceHeaders(http.Header(message.Header));defer nrnats.StartPublishSegment(txn,client.connection,message.Subject).End()};options:=make([]natsjs.PublishOpt,0,1);if messageID=strings.TrimSpace(messageID);messageID!=""{options=append(options,natsjs.WithMsgID(messageID))};if _,err:=client.jetStream.PublishMsg(ctx,message,options...);err!=nil{wrapped:=fmt.Errorf("publish JetStream message to %s: %w",message.Subject,err);if txn!=nil{txn.NoticeError(wrapped)};return wrapped};return nil}
func(client *Client)Ping(ctx context.Context)error{if client==nil||client.connection==nil||client.jetStream==nil||!client.connection.IsConnected(){return fmt.Errorf("JetStream client is not connected")};if _,err:=client.jetStream.AccountInfo(ctx);err!=nil{return fmt.Errorf("read JetStream account info: %w",err)};return nil}
func(client *Client)Close()error{if client==nil||client.connection==nil||client.connection.IsClosed(){return nil};if err:=client.connection.Drain();err!=nil{client.connection.Close();return fmt.Errorf("drain NATS connection: %w",err)};return nil}
