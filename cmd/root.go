package cmd

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	apiFlag     string
	dataFlag    string
	listFlag    bool
	versionFlag bool
	exampleFlag bool

	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"

	appLogger *zap.Logger
)

func Launch() (failed bool) {
	failed = false
	defer func() {
		if r := recover(); r != nil {
			failed = true
			errMsg := fmt.Sprintf("内部异常:%v", r)
			if appLogger != nil {
				appLogger.Error(errMsg, zap.String("api", apiFlag), zap.Any("panic", r), zap.Stack("stack"))
				_ = appLogger.Sync()
			}
			_ = JsonOutput(NotOk500(errMsg))
		}
	}()

	command := newCommand()
	err := command.Execute()

	if err != nil {
		if appErr, ok := err.(*AppError); ok {
			if appLogger != nil {
				appLogger.Error(appErr.Message,
					zap.Int("code", appErr.Code),
					zap.String("api", apiFlag),
					zap.Error(appErr.Cause),
					zap.String("stack", fmt.Sprintf("%+v", appErr)))
			}
			_ = JsonOutput(NotOk(appErr.Code, appErr.Message))
		} else {
			if appLogger != nil {
				appLogger.Error(fmt.Sprintf("%s", err), zap.String("api", apiFlag), zap.Stack("stack"))
			}
			_ = JsonOutput(NotOk500(fmt.Sprintf("%s", err)))
		}
		failed = true
	}
	return failed
}

func newCommand() *cobra.Command {
	command := &cobra.Command{
		Use:           "iss-open-cli",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !cmd.Flags().Changed("api") &&
				!cmd.Flags().Changed("data") &&
				!cmd.Flags().Changed("list") &&
				!cmd.Flags().Changed("version") &&
				!cmd.Flags().Changed("example") {
				return cmd.Help()
			}
			return run(cmd.Context())
		},
	}

	flags := command.Flags()
	flags.StringVar(&apiFlag, "api", "", "API 动作码，仅支持全名")
	flags.StringVar(&dataFlag, "data", "", "JSON 格式业务参数")
	flags.BoolVar(&listFlag, "list", false, "以表格形式打印支持的 API 列表")
	flags.BoolVarP(&versionFlag, "version", "v", false, "输出版本信息")
	flags.BoolVar(&exampleFlag, "example", false, "查看指定 API 的参数示例(需配合 --api 使用)")

	return command
}

func run(ctx context.Context) error {
	if versionFlag {
		fmt.Printf("version %s\n", version)
		fmt.Printf("commit: %s\n", commit)
		fmt.Printf("built at: %s\n", buildTime)
		return nil
	}

	if listFlag {
		printAPIList()
		return nil
	}

	if exampleFlag {
		if strings.TrimSpace(apiFlag) == "" {
			return Create400Error("查看参数示例需要指定 --api", nil)
		}
		return printAPIExample(apiFlag)
	}

	cfg, err := Load()
	if err != nil {
		return CreateAppError(E400, err.Error(), err)
	}

	loggers, err := NewLoggerFactory(cfg.Log.Dir, cfg.Log.Level)
	if err != nil {
		return Create500Error("初始化日志失败", err)
	}
	appLogger = loggers.App
	defer func() {
		_ = appLogger.Sync()
	}()

	if strings.TrimSpace(apiFlag) == "" || strings.TrimSpace(dataFlag) == "" {
		return Create400Error("缺少必填参数: --api 和 --data", nil)
	}

	if _, ok := GetAPIByCode(apiFlag); !ok {
		return Create400Error("--api 参数非法", nil)
	}

	payload := json.RawMessage(strings.TrimSpace(dataFlag))
	if !json.Valid(payload) {
		return Create400Error("--data 参数不是合法的 JSON 格式", nil)
	}

	appLogger.Info("开始执行开放平台API", zap.String("api", apiFlag))

	merchantOrderClient := NewClient(cfg, loggers)
	orderApplicationService := NewService(merchantOrderClient)

	result, err := orderApplicationService.Execute(ctx, apiFlag, payload)
	if err != nil {
		return err
	}

	appLogger.Info("开放平台API执行成功", zap.String("api", apiFlag))

	if err := JsonOutput(Ok(result)); err != nil {
		return fmt.Errorf("输出响应失败: %w", err)
	}
	return nil
}

func printAPIList() {
	fmt.Println("支持的 API 列表：")
	fmt.Printf("%-20s %-15s\n", "动作码", "中文名")
	fmt.Println(strings.Repeat("-", 80))

	for _, action := range APIActions {
		fmt.Printf("%-20s %-15s\n", action.Code, action.Name)
	}
	fmt.Println()
	fmt.Println("查看具体API的参数示例，请使用: iss-open-cli --api <动作码> --example")
}

func printAPIExample(apiCode string) error {
	action, found := GetAPIByCode(apiCode)
	if !found {
		return Create400Error(fmt.Sprintf("不支持的 API: %s", apiCode), nil)
	}

	fmt.Printf("API: %s (%s)\n\n", action.Code, action.Name)
	fmt.Println("Data示例:")
	fmt.Println(action.DataExample)
	return nil
}

const (
	E400 = 400
	E500 = 500
	E502 = 502

	defaultTimeout  = 3
	defaultLogLevel = "info"
	defaultLogDir   = "logs"
)

var (
	appRootDir     string
	appRootDirOnce sync.Once
)

type AppError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Cause   error  `json:"-"`
	stack   []uintptr
}

func NewAppError(code int, message string, cause error) *AppError {
	pc := make([]uintptr, 32)
	n := runtime.Callers(3, pc)
	return &AppError{Code: code, Message: message, Cause: cause, stack: pc[:n]}
}

func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("code=%d,message=%s,cause=%v", e.Code, e.Message, e.Cause)
	}
	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Cause
}

func (e *AppError) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			frames := runtime.CallersFrames(e.stack)
			for {
				frame, more := frames.Next()
				_, _ = fmt.Fprintf(s, "%s\n\t%s:%d\n", frame.Function, frame.File, frame.Line)
				if !more {
					break
				}
			}
		}
	}
}

func CreateAppError(code int, message string, cause error) *AppError {
	return NewAppError(code, message, cause)
}
func Create400Error(message string, cause error) *AppError {
	return CreateAppError(E400, "参数错误:"+message, cause)
}
func Create500Error(message string, cause error) *AppError {
	return CreateAppError(E500, message, cause)
}
func Create502Error(message string, cause error) *AppError {
	return CreateAppError(E502, "远程调用错误:"+message, cause)
}

type Response struct {
	Status  int    `json:"status"`
	Err     string `json:"err"`
	Success bool   `json:"success"`
	Data    any    `json:"data"`
}

func Ok(data any) Response {
	return Response{Status: 200, Success: true, Data: data}
}

func NotOk(status int, err string) Response {
	return Response{Status: status, Err: err, Success: false}
}

func NotOk500(err string) Response {
	return NotOk(500, err)
}

func JsonOutput(r Response) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(r); err != nil {
		return fmt.Errorf("输出响应失败: %w", err)
	}
	return nil
}

func GenerateSign(appSecret, clientID, shopID, timestamp string, data string) string {
	var builder strings.Builder

	builder.WriteString(appSecret)
	builder.WriteString("clientId")
	builder.WriteString(clientID)

	if data != "" {
		builder.WriteString("data")
		builder.WriteString(data)
	}

	builder.WriteString("shopId")
	builder.WriteString(shopID)
	builder.WriteString("timestamp")
	builder.WriteString(timestamp)

	sum := md5.Sum([]byte(builder.String()))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

type APIAction struct {
	Code        string
	Name        string
	DataExample string
}

var APIActions = []APIAction{
	{
		Code:        "orderCalculate",
		Name:        "订单询价",
		DataExample: `--api orderCalculate --data '{"cityName":"北京市","sender":{"fromAddress":"示例地址A","fromSenderName":"发件人","fromMobile":"13800000001","fromLatitude":"40.049058","fromLongitude":"116.379594"},"receiverList":[{"orderNo":"ORDER_2026041320001","toAddress":"示例地址B","toLatitude":"40.043612","toLongitude":"116.361199","toReceiverName":"收件人","toMobile":"13800000002"}]}'`,
	},
	{
		Code:        "orderPlace",
		Name:        "提交订单",
		DataExample: `{"issOrderNo":"TDH2026041300954053"}`,
	},
	{
		Code:        "orderInfo",
		Name:        "查询订单详情",
		DataExample: `{"issOrderNo":"TDH2026041300954053","thirdOrderNo":"OTK_2026041320001"}`,
	},
	{
		Code:        "abortOrder",
		Name:        "取消订单",
		DataExample: `{"issOrderNo":"TDH2026041300954053"}`,
	},
}

func GetAPIByCode(code string) (APIAction, bool) {
	for _, action := range APIActions {
		if action.Code == code {
			return action, true
		}
	}

	return APIAction{}, false
}

type Config struct {
	API  APIConfig  `mapstructure:"api"`
	Auth AuthConfig `mapstructure:"auth"`
	Log  LogConfig  `mapstructure:"log"`
}

type APIConfig struct {
	BaseURL string `mapstructure:"base_url"`
	Timeout int    `mapstructure:"timeout"`
}

type AuthConfig struct {
	ClientID  string `mapstructure:"client_id"`
	ShopID    string `mapstructure:"shop_id"`
	AppSecret string `mapstructure:"app_secret"`
}

type LogConfig struct {
	Dir   string
	Level string `mapstructure:"level"`
}

func Load() (Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetEnvPrefix("ISS_OPEN")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	_ = v.BindEnv("api.base_url", "ISS_OPEN_API_URL")
	_ = v.BindEnv("auth.client_id", "ISS_OPEN_AUTH_CLIENT_ID")
	_ = v.BindEnv("auth.shop_id", "ISS_OPEN_AUTH_SHOP_ID")
	_ = v.BindEnv("auth.app_secret", "ISS_OPEN_AUTH_APP_SECRET")
	_ = v.BindEnv("log.level", "ISS_OPEN_LOG_LEVEL")

	v.SetDefault("api.timeout", defaultTimeout)
	v.SetDefault("log.level", defaultLogLevel)

	configPaths := getConfigSearchPaths()
	configLoaded := false

	for _, configPath := range configPaths {
		if _, err := os.Stat(configPath); err == nil {
			v.SetConfigFile(configPath)
			if err := v.ReadInConfig(); err == nil {
				configLoaded = true
				break
			}
		}
	}

	if !configLoaded {
		return Config{}, fmt.Errorf("配置错误：未找到配置文件，请在以下位置之一放置 configs/config.yaml:\n%s",
			strings.Join(configPaths, "\n"))
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("配置错误：解析配置失败: %w", err)
	}

	if appRoot := GetAppRootDir(); appRoot != "" {
		cfg.Log.Dir = filepath.Join(appRoot, defaultLogDir)
	} else {
		cfg.Log.Dir = defaultLogDir
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func GetAppRootDir() string {
	appRootDirOnce.Do(func() {
		hasConfigs := func(dir string) bool {
			if dir == "" {
				return false
			}
			info, err := os.Stat(filepath.Join(dir, "configs"))
			return err == nil && info.IsDir()
		}

		if wd, _ := os.Getwd(); hasConfigs(wd) {
			appRootDir = wd
			return
		}

		if exePath, err := os.Executable(); err == nil {
			appRootDir = filepath.Dir(exePath)
		}
	})
	return appRootDir
}

func getConfigSearchPaths() []string {
	if appRoot := GetAppRootDir(); appRoot != "" {
		return []string{
			filepath.Join(appRoot, "configs", "config.yaml"),
			"configs/config.yaml",
		}
	}
	return []string{"configs/config.yaml"}
}

func (c Config) Validate() error {
	parsed, err := url.ParseRequestURI(c.API.BaseURL)
	if strings.TrimSpace(c.API.BaseURL) == "" {
		return fmt.Errorf("配置错误：api.base_url 不能为空")
	} else if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("配置错误：api.base_url 不是合法 URL")
	}

	if c.API.Timeout <= 0 {
		return fmt.Errorf("配置错误：api.timeout 必须大于 0")
	}

	required := []struct {
		name  string
		value string
	}{
		{"auth.client_id", c.Auth.ClientID},
		{"auth.shop_id", c.Auth.ShopID},
		{"auth.app_secret", c.Auth.AppSecret},
	}
	for _, item := range required {
		if strings.TrimSpace(item.value) == "" {
			return fmt.Errorf("配置错误：%s 不能为空", item.name)
		}
	}

	return nil
}

type Loggers struct {
	App *zap.Logger
}

type LoggerFactory struct {
	logDir   string
	logLevel string
}

func NewLoggerFactory(logDir, logLevel string) (*Loggers, error) {
	tid := uuid.New().String()

	factory := &LoggerFactory{
		logDir:   logDir,
		logLevel: logLevel,
	}

	if err := ensureLogDirectory(logDir); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %w", err)
	}

	appLogger, err := factory.createAppLogger(tid)
	if err != nil {
		return nil, fmt.Errorf("创建 app logger 失败: %w", err)
	}

	appLogger = appLogger.With(
		zap.Int("pid", os.Getpid()),
	)

	return &Loggers{
		App: appLogger,
	}, nil
}

func (f *LoggerFactory) createAppLogger(traceID string) (*zap.Logger, error) {
	zapLevel, err := parseZapLevel(f.logLevel)
	if err != nil {
		return nil, fmt.Errorf("无效的日志级别 %s: %w", f.logLevel, err)
	}

	encoder := f.createEncoder()

	appCore := zapcore.NewCore(
		encoder,
		zapcore.AddSync(f.rotateWriter("app.log")),
		zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
			return lvl >= zapLevel && lvl < zapcore.ErrorLevel
		}),
	)

	errorCore := zapcore.NewCore(
		encoder,
		zapcore.AddSync(f.rotateWriter("errors.log")),
		zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
			return lvl >= zapcore.ErrorLevel
		}),
	)

	core := zapcore.NewTee(appCore, errorCore)

	logger := zap.New(core, zap.AddCaller())
	logger = logger.With(zap.String("trace_id", traceID))

	return logger, nil
}

func (f *LoggerFactory) rotateWriter(filename string) *lumberjack.Logger {
	return &lumberjack.Logger{
		Filename:   filepath.Join(f.logDir, filename),
		MaxSize:    500,
		MaxBackups: 100,
		MaxAge:     10,
		Compress:   true,
		LocalTime:  true,
	}
}

func (f *LoggerFactory) createEncoder() zapcore.Encoder {
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:          "time",
		LevelKey:         "level",
		CallerKey:        "caller",
		MessageKey:       "msg",
		LineEnding:       zapcore.DefaultLineEnding,
		ConsoleSeparator: " | ",
		EncodeLevel:      zapcore.CapitalLevelEncoder,
		EncodeTime:       zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05"),
		EncodeCaller:     zapcore.ShortCallerEncoder,
	}

	return zapcore.NewConsoleEncoder(encoderConfig)
}

func parseZapLevel(level string) (zapcore.Level, error) {
	switch strings.ToLower(level) {
	case "debug":
		return zap.DebugLevel, nil
	case "info":
		return zap.InfoLevel, nil
	case "warn", "warning":
		return zap.WarnLevel, nil
	case "errors":
		return zap.ErrorLevel, nil
	case "fatal":
		return zap.FatalLevel, nil
	default:
		return zap.InfoLevel, fmt.Errorf("未知的日志级别: %s", level)
	}
}

func ensureLogDirectory(dirPath string) error {
	if dirPath == "." || dirPath == "" {
		return nil
	}

	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			return err
		}
	}
	return nil
}

type OrderCalculateRequest struct {
	CityName     string         `json:"cityName"`
	Sender       SenderInfo     `json:"sender"`
	ReceiverList []ReceiverInfo `json:"receiverList"`
	AppointType  int            `json:"appointType"`
}

type SenderInfo struct {
	FromAddress       string `json:"fromAddress"`
	FromAddressDetail string `json:"fromAddressDetail"`
	FromSenderName    string `json:"fromSenderName"`
	FromMobile        string `json:"fromMobile"`
	FromLatitude      string `json:"fromLatitude"`
	FromLongitude     string `json:"fromLongitude"`
}

type ReceiverInfo struct {
	OrderNo         string `json:"orderNo"`
	ToAddress       string `json:"toAddress"`
	ToAddressDetail string `json:"toAddressDetail"`
	ToReceiverName  string `json:"toReceiverName"`
	ToMobile        string `json:"toMobile"`
	ToLatitude      string `json:"toLatitude"`
	ToLongitude     string `json:"toLongitude"`
	GoodType        int    `json:"goodType"`
	Weight          int    `json:"weight"`
}

type OrderCalculateResponse struct {
	TotalDistance         int       `json:"totalDistance"`
	TotalWeight           int       `json:"totalWeight"`
	OrderNumber           string    `json:"orderNumber"`
	FeeInfoList           []FeeInfo `json:"feeInfoList"`
	TotalAmount           int       `json:"totalAmount"`
	CouponSaveFee         int       `json:"couponSaveFee"`
	TotalFeeAfterSave     int       `json:"totalFeeAfterSave"`
	EstimateGrabSecond    int       `json:"estimateGrabSecond"`
	EstimateReceiveSecond int       `json:"estimateReceiveSecond"`
}

type FeeInfo struct {
	Type int    `json:"type"`
	Des  string `json:"des"`
	Fee  int    `json:"fee"`
}

type OrderPlaceRequest struct {
	IssOrderNo string `json:"issOrderNo"`
}

type OrderPlaceResponse struct {
	TotalDistance         int               `json:"totalDistance"`
	TotalWeight           int               `json:"totalWeight"`
	OrderNumber           string            `json:"orderNumber"`
	FeeInfoList           []FeeInfo         `json:"feeInfoList"`
	TotalAmount           int               `json:"totalAmount"`
	CouponSaveFee         int               `json:"couponSaveFee"`
	DynamicSubsidyAmount  int               `json:"dynamicSubsidyAmount"`
	TotalFeeAfterSave     int               `json:"totalFeeAfterSave"`
	ExpectReceiveTime     *string           `json:"expectReceiveTime,omitempty"`
	EstimateGrabSecond    int               `json:"estimateGrabSecond"`
	EstimateReceiveSecond int               `json:"estimateReceiveSecond"`
	InterestDtoList       []InterestDtoItem `json:"interestDtoList"`
	OrderInfoList         []OrderInfoItem   `json:"orderInfoList"`
}

type OrderInfoItem struct {
	OrderNo            string `json:"orderNo"`
	OrderingSourceType *int   `json:"orderingSourceType,omitempty"`
	OrderingSourceNo   string `json:"orderingSourceNo,omitempty"`
}

type InterestDtoItem struct {
	Type    int    `json:"type"`
	Desc    string `json:"desc"`
	Status  int    `json:"status"`
	SubDesc string `json:"subDesc,omitempty"`
}

type OrderInfoRequest struct {
	IssOrderNo   string `json:"issOrderNo"`
	ThirdOrderNo string `json:"thirdOrderNo"`
}

type OrderInfoResponse struct {
	OrderNo                    string            `json:"orderNo"`
	OrderStatus                int               `json:"orderStatus"`
	OrderStatusDesc            string            `json:"orderStatusDesc"`
	SubOrderStatus             int               `json:"subOrderStatus"`
	SubOrderStatusDesc         string            `json:"subOrderStatusDesc"`
	PickupPassword             *string           `json:"pickupPassword,omitempty"`
	DeliveryPassword           *string           `json:"deliveryPassword,omitempty"`
	Courier                    *CourierInfo      `json:"courier,omitempty"`
	StatusChangeLog            []StatusChangeLog `json:"statusChangeLog"`
	AbortInfo                  *AbortInfo        `json:"abortInfo,omitempty"`
	CourierPickupOrderPhotos   []string          `json:"courierPickupOrderPhotos,omitempty"`
	CourierDeliveryOrderPhotos []string          `json:"courierDeliveryOrderPhotos,omitempty"`
	FeeInfoList                []FeeInfo         `json:"feeInfoList,omitempty"`
	TotalAmount                int               `json:"totalAmount"`
	CouponSaveFee              int               `json:"couponSaveFee"`
	TotalFeeAfterSave          int               `json:"totalFeeAfterSave"`
	ActiveGold                 *int              `json:"activeGold,omitempty"`
	InterestDtoList            []InterestDtoItem `json:"interestDtoList,omitempty"`
	Drawback                   int               `json:"drawback"`
	SendBackFee                int               `json:"sendBackFee"`
}

type CourierInfo struct {
	Latitude                *string                `json:"latitude,omitempty"`
	Longitude               *string                `json:"longitude,omitempty"`
	Name                    *string                `json:"name,omitempty"`
	Mobile                  *string                `json:"mobile,omitempty"`
	Time                    *string                `json:"time,omitempty"`
	Type                    *int                   `json:"type,omitempty"`
	OrderCount              *int                   `json:"orderCount,omitempty"`
	HeadIcon                *string                `json:"headIcon,omitempty"`
	ID                      *string                `json:"id,omitempty"`
	Blacklisted             *int                   `json:"blacklisted,omitempty"`
	Gcj02Lng                *string                `json:"gcj02Lng,omitempty"`
	Gcj02Lat                *string                `json:"gcj02Lat,omitempty"`
	DeliveryProcessTrail    []DeliveryProcessTrail `json:"deliveryProcessTrail,omitempty"`
	Gcj02Trail              []map[string]any       `json:"gcj02Trail,omitempty"`
	EstimateDeliveryTimeTip *string                `json:"estimateDeliveryTimeTip,omitempty"`
	PickupTimeout           *string                `json:"pickupTimeout,omitempty"`
}

type DeliveryProcessTrail struct {
	Longitude string `json:"longitude"`
	Latitude  string `json:"latitude"`
	Datetime  string `json:"datetime"`
}

type StatusChangeLog struct {
	OrderStatus int    `json:"orderStatus"`
	UpdateTime  string `json:"updateTime"`
}

type AbortInfo struct {
	DeductAmount *int   `json:"deductAmount,omitempty"`
	AbortType    *int   `json:"abortType,omitempty"`
	PunishType   *int   `json:"punishType,omitempty"`
	AbortReason  string `json:"abortReason,omitempty"`
}

type AbortOrderRequest struct {
	IssOrderNo string `json:"issOrderNo"`
}

type AbortOrderResponse struct {
	SendBackFee  int    `json:"sendBackFee"`
	DeductAmount int    `json:"deductAmount"`
	AbortType    int    `json:"abortType"`
	PunishType   *int   `json:"punishType,omitempty"`
	AbortReason  string `json:"abortReason"`
}

type OpenResponse struct {
	Status int             `json:"status"`
	Msg    string          `json:"msg"`
	Data   json.RawMessage `json:"data"`
}

type MerchantOrderClient struct {
	baseURL    string
	clientID   string
	shopID     string
	appSecret  string
	httpClient *http.Client
	loggers    *Loggers
}

func NewClient(cfg Config, loggers *Loggers) *MerchantOrderClient {
	transport := &http.Transport{
		TLSHandshakeTimeout: 10 * time.Second,
	}

	return &MerchantOrderClient{
		baseURL:   strings.TrimRight(cfg.API.BaseURL, "/"),
		clientID:  cfg.Auth.ClientID,
		shopID:    cfg.Auth.ShopID,
		appSecret: cfg.Auth.AppSecret,
		httpClient: &http.Client{
			Timeout:   time.Duration(cfg.API.Timeout) * time.Second,
			Transport: transport,
		},
		loggers: loggers,
	}
}

func (c *MerchantOrderClient) Call(ctx context.Context, path string, payload json.RawMessage) (json.RawMessage, error) {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	data := compactJSON(payload)
	sign := GenerateSign(c.appSecret, c.clientID, c.shopID, timestamp, data)

	form := url.Values{}
	form.Set("clientId", c.clientID)
	form.Set("shopId", c.shopID)
	form.Set("timestamp", timestamp)
	form.Set("sign", sign)
	if data != "" {
		form.Set("data", data)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewBufferString(form.Encode()))
	if err != nil {
		return nil, Create502Error("构造请求失败", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	c.loggers.App.Debug("发送开放平台请求",
		zap.String("method", "POST"),
		zap.String("path", path),
		zap.String("timestamp", timestamp))

	startTime := time.Now()
	resp, err := c.httpClient.Do(req)
	duration := time.Since(startTime)

	if err != nil {
		c.loggers.App.Error("HTTP请求失败",
			zap.String("path", path),
			zap.Int64("duration_ms", duration.Milliseconds()),
			zap.Error(err))
		return nil, Create502Error("请求失败", err)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.loggers.App.Warn("关闭HTTP响应体失败", zap.String("path", path), zap.Error(err))
		}
	}()

	c.loggers.App.Info("HTTP请求完成",
		zap.String("path", path),
		zap.Int("status", resp.StatusCode),
		zap.Int64("duration_ms", duration.Milliseconds()))

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.loggers.App.Error("读取开放平台响应失败",
			zap.String("path", path),
			zap.Error(err))
		return nil, Create502Error("读取响应失败", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		c.loggers.App.Warn("开放平台HTTP状态码异常",
			zap.String("path", path),
			zap.Int("status", resp.StatusCode),
			zap.String("response_body", responseBodySnippet(body)))
		return nil, Create502Error(fmt.Sprintf("HTTP 状态码异常: %d", resp.StatusCode), nil)
	}

	if len(bytes.TrimSpace(body)) == 0 {
		c.loggers.App.Warn("开放平台响应为空", zap.String("path", path))
		return nil, Create502Error("平台响应为空", nil)
	}

	var openResp OpenResponse
	if err := json.Unmarshal(body, &openResp); err != nil {
		c.loggers.App.Warn("开放平台响应不是合法JSON",
			zap.String("path", path),
			zap.String("response_body", responseBodySnippet(body)),
			zap.Error(err))
		return nil, Create502Error("平台响应不是合法 JSON", err)
	}

	if openResp.Status != 200 {
		msg := strings.TrimSpace(openResp.Msg)
		if msg == "" {
			msg = "开放平台返回失败"
		}
		c.loggers.App.Warn("开放平台业务状态失败",
			zap.String("path", path),
			zap.Int("platform_status", openResp.Status),
			zap.String("platform_msg", msg))
		return nil, Create502Error(fmt.Sprintf("平台返回失败：%s", msg), nil)
	}

	if len(openResp.Data) == 0 || string(openResp.Data) == "null" {
		return []byte(`{}`), nil
	}

	return openResp.Data, nil
}

func compactJSON(payload json.RawMessage) string {
	if len(bytes.TrimSpace(payload)) == 0 {
		return ""
	}

	buffer := bytes.NewBuffer(nil)
	if err := json.Compact(buffer, payload); err != nil {
		return string(payload)
	}

	return buffer.String()
}

func responseBodySnippet(body []byte) string {
	const maxResponseBodyLogBytes = 2048

	body = bytes.TrimSpace(body)
	if len(body) <= maxResponseBodyLogBytes {
		return string(body)
	}

	return string(body[:maxResponseBodyLogBytes]) + "...(truncated)"
}

func callPlatform[T any](c *MerchantOrderClient, ctx context.Context, path string, req any) (*T, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, Create500Error("内部错误：序列化请求参数失败", err)
	}

	rawData, err := c.Call(ctx, path, data)
	if err != nil {
		return nil, err
	}

	var resp T
	if err := json.Unmarshal(rawData, &resp); err != nil {
		c.loggers.App.Warn("解析平台响应数据失败",
			zap.String("path", path),
			zap.String("response_data", responseBodySnippet(rawData)),
			zap.Error(err))
		return nil, Create502Error("解析平台响应数据失败", err)
	}

	return &resp, nil
}

type OrderApplicationService struct {
	merchantOrderClient *MerchantOrderClient
}

func NewService(merchantOrderClient *MerchantOrderClient) *OrderApplicationService {
	return &OrderApplicationService{merchantOrderClient: merchantOrderClient}
}

type orderHandler func(context.Context, json.RawMessage) (map[string]any, error)

func (s *OrderApplicationService) Execute(ctx context.Context, apiCode string, payload json.RawMessage) (map[string]any, error) {
	handlers := map[string]orderHandler{
		"orderCalculate": s.handleOrderCalculate,
		"orderPlace":     s.handleOrderPlace,
		"orderInfo":      s.handleOrderInfo,
		"abortOrder":     s.handleAbortOrder,
	}
	handler, ok := handlers[apiCode]
	if !ok {
		return nil, Create400Error("--api 参数非法", nil)
	}
	return handler(ctx, payload)
}

func (s *OrderApplicationService) handleOrderCalculate(ctx context.Context, payload json.RawMessage) (map[string]any, error) {
	req, err := decodeRequest[OrderCalculateRequest](payload)
	if err != nil {
		return nil, err
	}
	receiverList := req.ReceiverList
	if receiverList != nil && len(receiverList) > 0 {
		receiverList[0].GoodType = 10
		receiverList[0].Weight = 1
	}
	req.AppointType = 0
	resp, err := callPlatform[OrderCalculateResponse](s.merchantOrderClient, ctx, "/openapi/merchants/v5/orderCalculate", req)
	if err != nil {
		return nil, err
	}

	return convertToMap(resp)
}

func (s *OrderApplicationService) handleOrderPlace(ctx context.Context, payload json.RawMessage) (map[string]any, error) {
	return handlePlatform[OrderPlaceRequest, OrderPlaceResponse](ctx, s.merchantOrderClient, payload, "/openapi/merchants/v5/orderPlace")
}

func (s *OrderApplicationService) handleOrderInfo(ctx context.Context, payload json.RawMessage) (map[string]any, error) {
	return handlePlatform[OrderInfoRequest, OrderInfoResponse](ctx, s.merchantOrderClient, payload, "/openapi/merchants/v5/orderInfo")
}

func (s *OrderApplicationService) handleAbortOrder(ctx context.Context, payload json.RawMessage) (map[string]any, error) {
	return handlePlatform[AbortOrderRequest, AbortOrderResponse](ctx, s.merchantOrderClient, payload, "/openapi/merchants/v5/abortOrder")
}

func decodeRequest[T any](payload json.RawMessage) (T, error) {
	var req T
	if err := json.Unmarshal(payload, &req); err != nil {
		return req, Create400Error("解析请求参数失败", err)
	}
	return req, nil
}

func handlePlatform[Req any, Resp any](ctx context.Context, client *MerchantOrderClient, payload json.RawMessage, path string) (map[string]any, error) {
	req, err := decodeRequest[Req](payload)
	if err != nil {
		return nil, err
	}
	resp, err := callPlatform[Resp](client, ctx, path, req)
	if err != nil {
		return nil, err
	}
	return convertToMap(resp)
}

func convertToMap(data any) (map[string]any, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, Create500Error("内部错误：序列化响应数据失败", err)
	}

	var result map[string]any
	if err := json.Unmarshal(jsonData, &result); err != nil {
		return nil, Create500Error("内部错误：反序列化响应数据失败", err)
	}

	return result, nil
}
