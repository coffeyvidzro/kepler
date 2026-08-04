package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	sentryecho "github.com/getsentry/sentry-go/echo"
	"github.com/labstack/echo/v5"

	awsses "github.com/coffeyvidzro/dugble/server/internal/adapters/amazon/ses"
	awssns "github.com/coffeyvidzro/dugble/server/internal/adapters/amazon/sns"
	"github.com/coffeyvidzro/dugble/server/internal/adapters/dns/netdns"
	"github.com/coffeyvidzro/dugble/server/internal/adapters/postgres"
	redisadapter "github.com/coffeyvidzro/dugble/server/internal/adapters/redis"
	arcjetadapter "github.com/coffeyvidzro/dugble/server/internal/adapters/security/arcjet"
	"github.com/coffeyvidzro/dugble/server/internal/config"
	"github.com/coffeyvidzro/dugble/server/internal/delivery/email/feedback"
	emaildelivery "github.com/coffeyvidzro/dugble/server/internal/delivery/email/send"
	systememail "github.com/coffeyvidzro/dugble/server/internal/delivery/email/system"
	smsdelivery "github.com/coffeyvidzro/dugble/server/internal/delivery/sms"
	verifydispatch "github.com/coffeyvidzro/dugble/server/internal/delivery/verify/dispatch"
	smsintegration "github.com/coffeyvidzro/dugble/server/internal/integration/sms"
	"github.com/coffeyvidzro/dugble/server/internal/integration/sms/provider/arkesel"
	"github.com/coffeyvidzro/dugble/server/internal/integration/sms/provider/celcom"
	"github.com/coffeyvidzro/dugble/server/internal/integration/sms/provider/mnotify"
	"github.com/coffeyvidzro/dugble/server/internal/integration/sms/routing"
	"github.com/coffeyvidzro/dugble/server/internal/messaging/outbox"
	"github.com/coffeyvidzro/dugble/server/internal/modules/auditevent"
	"github.com/coffeyvidzro/dugble/server/internal/modules/auth"
	"github.com/coffeyvidzro/dugble/server/internal/modules/domain"
	emailmodule "github.com/coffeyvidzro/dugble/server/internal/modules/email"
	"github.com/coffeyvidzro/dugble/server/internal/modules/emailtenant"
	"github.com/coffeyvidzro/dugble/server/internal/modules/mfa"
	"github.com/coffeyvidzro/dugble/server/internal/modules/senderid"
	"github.com/coffeyvidzro/dugble/server/internal/modules/session"
	smsmodule "github.com/coffeyvidzro/dugble/server/internal/modules/sms"
	"github.com/coffeyvidzro/dugble/server/internal/modules/team"
	"github.com/coffeyvidzro/dugble/server/internal/modules/teamtoken"
	"github.com/coffeyvidzro/dugble/server/internal/modules/user"
	verifymodule "github.com/coffeyvidzro/dugble/server/internal/modules/verify"
	"github.com/coffeyvidzro/dugble/server/internal/modules/wallet"
	"github.com/coffeyvidzro/dugble/server/internal/modules/webhooks"
	"github.com/coffeyvidzro/dugble/server/internal/monitoring"
	"github.com/coffeyvidzro/dugble/server/internal/platform/audit"
	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
	platformbilling "github.com/coffeyvidzro/dugble/server/internal/platform/billing"
	platformemail "github.com/coffeyvidzro/dugble/server/internal/platform/email"
	"github.com/coffeyvidzro/dugble/server/internal/platform/idempotency"
	"github.com/coffeyvidzro/dugble/server/internal/platform/systemmail"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	platformwebhook "github.com/coffeyvidzro/dugble/server/internal/platform/webhook"
	httptransport "github.com/coffeyvidzro/dugble/server/internal/transport/http"
	emailhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/email"
	healthhttp "github.com/coffeyvidzro/dugble/server/internal/transport/http/health"
	httpmiddleware "github.com/coffeyvidzro/dugble/server/internal/transport/http/middleware"
	providersns "github.com/coffeyvidzro/dugble/server/internal/transport/http/provider/aws/sns"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
)

// Wire builds the server and returns a cleanup function for all initialized resources.
func Wire(ctx context.Context) (*Application, func(), error) {
	if ctx == nil {
		return nil, nil, errors.New("server wiring context is required")
	}
	cleanups := &cleanupStack{}
	fail := func(err error) (*Application, func(), error) {
		cleanups.Run()
		return nil, nil, err
	}
	cfg, err := config.Load()
	if err != nil { return fail(fmt.Errorf("load configuration: %w", err)) }
	if err := monitoring.InitSentry(cfg.Sentry, cfg.AppEnv); err != nil { return fail(fmt.Errorf("initialize Sentry: %w", err)) }
	cleanups.Add(func(){ monitoring.FlushSentry(5*time.Second) })
	newRelic, err := monitoring.NewRelic("dugble-api", cfg.AppEnv, cfg.NewRelic)
	if err != nil { return fail(fmt.Errorf("initialize New Relic: %w", err)) }
	cleanups.Add(func(){ monitoring.Shutdown(newRelic, 5*time.Second) })
	startupCtx, cancelStartup := context.WithTimeout(ctx, 15*time.Second)
	defer cancelStartup()
	db, err := postgres.New(startupCtx, cfg.DatabaseURL)
	if err != nil { return fail(fmt.Errorf("initialize PostgreSQL: %w", err)) }
	cleanups.Add(db.Close)
	redisClient, err := redisadapter.New(startupCtx, cfg.RedisURL)
	if err != nil { return fail(fmt.Errorf("initialize Redis: %w", err)) }
	cleanups.Add(func(){ if closeErr:=redisClient.Close(); closeErr!=nil { slog.Warn("close Redis client", "error", closeErr) } })
	arcjetClient, err := arcjetadapter.New(cfg.ArcjetKey)
	if err != nil { return fail(fmt.Errorf("initialize Arcjet: %w", err)) }
	renderer, err := systemmail.NewRenderer()
	if err != nil { return fail(fmt.Errorf("initialize email renderer: %w", err)) }
	emailClient, err := awsses.NewClient(cfg.AWS.Region, cfg.AWS.FromEmail, cfg.AWS.AccessKey, cfg.AWS.SecretKey, cfg.AWS.SESTransactionalConfigurationSet)
	if err != nil { return fail(fmt.Errorf("initialize SES email client: %w", err)) }
	outboxRepository := outbox.NewRepository(db)
	systemEmailQueue := systememail.NewQueue(outboxRepository, platformemail.Message{Provider:awsses.ProviderSES,Region:cfg.AWS.Region,Stream:"transactional",ConfigurationSet:cfg.AWS.SESTransactionalConfigurationSet,SESTenantName:cfg.AWS.SESTenantName})

	var snsHandler *providersns.Handler
	if len(cfg.AWS.SNSTopicARNs)>0 {
		certificateLoader:=awssns.NewHTTPCertificateLoader(nil)
		verifier:=awssns.NewVerifier(cfg.AWS.SNSTopicARNs,certificateLoader)
		confirmer:=awssns.NewConfirmer(awssns.NewHTTPConfirmSubscriptionClient(nil))
		ingestor:=feedback.NewRepository(db,outboxRepository)
		snsHandler=providersns.NewHandler(verifier,confirmer,ingestor)
	}

	smsRouter,err:=routing.NewService(routing.DefaultConfig(),arkesel.NewProvider(arkesel.NewClient(cfg.Arkesel)),celcom.NewProvider(celcom.NewClient(cfg.Celcom)),mnotify.NewProvider(mnotify.NewClient(cfg.MNotify)))
	if err!=nil{return fail(fmt.Errorf("initialize SMS router: %w",err))}
	smsSender,err:=smsintegration.NewService(smsRouter)
	if err!=nil{return fail(fmt.Errorf("initialize SMS sender: %w",err))}

	notificationEmailService:=systemmail.NewEmailService(systemEmailQueue,renderer,cfg.FrontendURL,cfg.AWS.FromEmail)
	auditRepository:=audit.NewRepository(db)
	audit.SetSink(auditRepository)
	sessionRepository:=session.NewRepository(db)
	authRepository:=auth.NewRepository(db)
	mfaCipher,err:=authnz.NewSecretCipherKeyring(cfg.EncryptionKeys)
	if err!=nil{return fail(fmt.Errorf("initialize MFA cipher: %w",err))}
	mfaService:=mfa.NewService(mfa.NewRepository(db),mfaCipher,"Dugble").WithNotifier(notificationEmailService)
	authService:=auth.NewService(authRepository,sessionRepository,notificationEmailService,mfaService)
	userRepository:=user.NewRepository(db)
	mfaService.WithRecipientStore(userRepository)
	teamRepository:=team.NewRepository(db)
	teamService:=team.NewService(teamRepository,notificationEmailService).WithRecipientStore(userRepository)
	teamTokenRepository:=teamtoken.NewRepository(db)
	domainRepository:=domain.NewRepository(db)
	emailTenantRepository:=emailtenant.NewRepository(db)
	emailTenantService:=emailtenant.NewService(emailTenantRepository,emailtenant.NewProvisionQueue(outboxRepository))
	senderIDRepository:=senderid.NewRepository(db)
	webhookRepository:=webhooks.NewRepository(db)
	webhookEmitter:=platformwebhook.NewEmitter(webhookRepository)
	productRuntime,err:=New(Dependencies{WebhookEmitter:webhookEmitter})
	if err!=nil{return fail(fmt.Errorf("initialize product runtime: %w",err))}
	smsRepository:=smsmodule.NewRepositoryWithWebhookEmitter(db,webhookEmitter)
	billingService:=platformbilling.NewService(platformbilling.NewRepository(db))
	smsService:=smsmodule.NewService(smsRepository,smsSender,smsdelivery.NewQueue(outboxRepository),billingService)
	emailRepository:=emailmodule.NewRepository(db)
	emailAPIService:=emailmodule.NewService(emailRepository,emaildelivery.NewQueue(outboxRepository),emailmodule.ServiceConfig{DefaultFromEmail:platformemail.CustomerOnboardingIdentity,DefaultProvider:domain.DefaultProvider,DefaultRegion:cfg.AWS.Region},billingService)
	verifySecret:=[]byte(cfg.Verify.HMACSecret)
	verifyCodes,err:=verifymodule.NewCodeManager(verifySecret,mfaCipher)
	if err!=nil{return fail(fmt.Errorf("initialize verify code manager: %w",err))}
	verifyAbuse,err:=verifymodule.NewRedisAbuseControls(redisClient,verifySecret,verifymodule.DefaultAbusePolicy())
	if err!=nil{return fail(fmt.Errorf("initialize verify abuse controls: %w",err))}
	verifyService:=verifymodule.NewService(verifymodule.NewRepository(db),verifyCodes,verifydispatch.NewQueue(outboxRepository),productRuntime.Events).WithAbuseControls(verifyAbuse)
	webhookService:=webhooks.NewService(webhookRepository,webhookEmitter)

	authMiddleware:=httpmiddleware.SessionAuth(httpmiddleware.SessionAuthConfig{Sessions:sessionRepository,Users:authRepository})
	csrfConfig:=httpmiddleware.CSRFConfig{Development:cfg.IsDevelopment(),TrustedOrigins:cfg.CORSOrigins}
	csrfMiddleware:=httpmiddleware.CSRF(csrfConfig)
	tenantMiddleware:=func(permission tenant.Permission) echo.MiddlewareFunc{return httpmiddleware.Tenant(httpmiddleware.TenantConfig{Memberships:teamRepository,Required:permission})}
	tenantAccess:=func(permission tenant.Permission) echo.MiddlewareFunc{return httpmiddleware.TenantAccess(httpmiddleware.TenantAccessConfig{Sessions:sessionRepository,Users:authRepository,Memberships:teamRepository,Tokens:teamTokenRepository,CSRF:csrfConfig,Required:permission})}

	registrar:=func(router *echo.Echo) error {
		healthhttp.RegisterRoutes(router,healthhttp.NewHandler(db,redisClient))
		if snsHandler!=nil{providersns.RegisterRoutes(router,snsHandler)}
		router.GET("/csrf",func(c *echo.Context) error{token,ok:=c.Get(httpmiddleware.CSRFContextKey).(string);if !ok||token==""{return httptransport.Error(c,apperrors.NewInternal("CSRF token is not available",nil))};return httptransport.OK(c,map[string]string{"csrf_token":token})},csrfMiddleware)
		auth.RegisterRoutes(router,auth.NewHandler(authService,cfg.IsDevelopment(),cfg.CookieDomain),authMiddleware,csrfMiddleware)
		mfa.RegisterRoutes(router,mfa.NewHandler(mfaService),authMiddleware,csrfMiddleware)
		user.RegisterRoutes(router,user.NewHandler(user.NewService(userRepository,notificationEmailService)),authMiddleware,csrfMiddleware)
		team.RegisterRoutes(router,team.NewHandler(teamService),authMiddleware,csrfMiddleware,tenantMiddleware)
		wallet.RegisterRoutes(router,wallet.NewHandler(wallet.NewService(wallet.NewRepository(db))),tenantAccess)
		auditevent.RegisterRoutes(router,auditevent.NewHandler(auditevent.NewService(auditRepository)),authMiddleware,csrfMiddleware,tenantMiddleware)
		teamtoken.RegisterRoutes(router,teamtoken.NewHandler(teamtoken.NewService(teamTokenRepository).WithNotifier(notificationEmailService)),authMiddleware,csrfMiddleware,tenantMiddleware)
		senderid.RegisterRoutes(router,senderid.NewHandler(senderid.NewService(senderIDRepository)),tenantAccess)
		domain.RegisterRoutes(router,domain.NewHandler(domain.NewService(domainRepository,emailClient,netdns.New(),emailTenantService)),tenantAccess)
		smsmodule.RegisterRoutes(router,smsmodule.NewHandler(smsService),tenantAccess)
		emailhttp.RegisterRoutes(router,emailhttp.NewHandler(emailAPIService),tenantAccess)
		verifymodule.RegisterRoutes(router,verifymodule.NewHandler(verifyService),tenantAccess)
		webhooks.RegisterRoutes(router,webhooks.NewHandler(webhookService),authMiddleware,csrfMiddleware,tenantMiddleware)
		session.RegisterRoutes(router,session.NewHandler(session.NewService(sessionRepository)),authMiddleware,csrfMiddleware)
		return nil
	}

	router,err:=httptransport.NewRouter(httptransport.RouterConfig{
		Development:cfg.IsDevelopment(),CORSOrigins:cfg.CORSOrigins,Arcjet:arcjetClient,BodyLimit:platformemail.MaxHTTPRequestBytes,
		Idempotency:httpmiddleware.IdempotencyConfig{Repository:idempotency.NewRepository(db)},
		Middleware:[]echo.MiddlewareFunc{httpmiddleware.NewRelic(),sentryecho.New(sentryecho.Options{Repanic:true,WaitForDelivery:false}),httpmiddleware.SentryErrors()},
	},registrar)
	if err!=nil{return fail(fmt.Errorf("create HTTP router: %w",err))}
	application,err:=NewApplication(monitoring.WrapHTTP(newRelic,router),":"+cfg.HTTPPort)
	if err!=nil{return fail(fmt.Errorf("create HTTP application: %w",err))}
	return application,cleanups.Run,nil
}

type cleanupStack struct{functions []func()}
func(stack *cleanupStack)Add(cleanup func()){if stack==nil||cleanup==nil{return};stack.functions=append(stack.functions,cleanup)}
func(stack *cleanupStack)Run(){if stack==nil{return};for index:=len(stack.functions)-1;index>=0;index--{stack.functions[index]()};stack.functions=nil}
