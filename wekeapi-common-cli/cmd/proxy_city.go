// 本地省市代码查询命令：平台无对应 API，数据内置自 wekeapi 官方文档"城市代码对照表"。
// 平台代理（proxyType=platform）的 proxyInfo 需传城市代码，这两个命令用于查询可用省市及代码。
package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"wekeapi-cli/internal/runner"
)

// provinceCitiesText 城市代码对照表，来自官方文档（登录模块参考-城市代码对照表）：
// https://doc.wekeapi.com/%E7%99%BB%E5%BD%95%E6%A8%A1%E5%9D%97%E5%8F%82%E8%80%83-8859316m0
// 格式：每行"省份:城市1-代码1,城市2-代码2,..."。上海不在平台对照表内。
const provinceCitiesText = `北京:北京-11
天津:天津-12
河北:石家庄-1301,唐山-1302,秦皇岛-1303,邯郸-1304,邢台-1305,保定-1306,张家口-1307,承德-1308,沧州-1309,廊坊-1310,衡水-1311
山西:太原-1401,大同-1402,阳泉-1403,长治-1404,晋城-1405,朔州-1406,晋中-1407,运城-1408,忻州-1409,临汾-1410,吕梁-1411
内蒙古:呼和浩特-1501,包头-1502,乌海-1503,赤峰-1504,通辽-1505,鄂尔多斯-1506,呼伦贝尔-1507,巴彦淖尔-1508,乌兰察布-1509,兴安盟-1522,锡林郭勒-1525,阿拉善-1529
辽宁:沈阳-2101,大连-2102,鞍山-2103,抚顺-2104,本溪-2105,丹东-2106,锦州-2107,营口-2108,阜新-2109,辽阳-2110,盘锦-2111,铁岭-2112,朝阳-2113,葫芦岛-2114
吉林:长春-2201,吉林-2202,四平-2203,辽源-2204,通化-2205,白山-2206,松原-2207,白城-2208,延边-2224
黑龙江:哈尔滨-2301,齐齐哈尔-2302,鸡西-2303,鹤岗-2304,双鸭山-2305,大庆-2306,伊春-2307,佳木斯-2308,七台河-2309,牡丹江-2310,黑河-2311,绥化-2312,大兴安岭-2327
江苏:南京-3201,无锡-3202,徐州-3203,常州-3204,苏州-3205,南通-3206,连云港-3207,淮安-3208,盐城-3209,扬州-3210,镇江-3211,泰州-3212,宿迁-3213
浙江:杭州-3301,宁波-3302,温州-3303,嘉兴-3304,湖州-3305,绍兴-3306,金华-3307,衢州-3308,舟山-3309,台州-3310,丽水-3311
安徽:合肥-3401,芜湖-3402,蚌埠-3403,淮南-3404,马鞍山-3405,淮北-3406,铜陵-3407,安庆-3408,黄山-3410,滁州-3411,阜阳-3412,宿州-3413,六安-3415,亳州-3416,池州-3417,宣城-3418
福建:福州-3501,厦门-3502,莆田-3503,三明-3504,泉州-3505,漳州-3506,南平-3507,龙岩-3508,宁德-3509
江西:南昌-3601,景德镇-3602,萍乡-3603,九江-3604,新余-3605,鹰潭-3606,赣州-3607,吉安-3608,宜春-3609,抚州-3610,上饶-3611
山东:济南-3701,青岛-3702,淄博-3703,枣庄-3704,东营-3705,烟台-3706,潍坊-3707,济宁-3708,泰安-3709,威海-3710,日照-3711,临沂-3713,德州-3714,聊城-3715,滨州-3716,菏泽-3717
河南:郑州-4101,开封-4102,洛阳-4103,平顶山-4104,安阳-4105,鹤壁-4106,新乡-4107,焦作-4108,濮阳-4109,许昌-4110,漯河-4111,三门峡-4112,南阳-4113,商丘-4114,信阳-4115,周口-4116,驻马店-4117
湖北:武汉-4201,黄石-4202,十堰-4203,宜昌-4205,襄阳-4206,鄂州-4207,荆门-4208,孝感-4209,荆州-4210,黄冈-4211,咸宁-4212,随州-4213,恩施-4228
湖南:长沙-4301,株洲-4302,湘潭-4303,衡阳-4304,邵阳-4305,岳阳-4306,常德-4307,张家界-4308,益阳-4309,郴州-4310,永州-4311,怀化-4312,娄底-4313,湘西-4331
广东:广州-4401,韶关-4402,深圳-4403,珠海-4404,汕头-4405,佛山-4406,江门-4407,湛江-4408,茂名-4409,肇庆-4412,惠州-4413,梅州-4414,汕尾-4415,河源-4416,阳江-4417,清远-4418,东莞-4419,中山-4420,潮州-4451,揭阳-4452,云浮-4453
广西:南宁-4501,柳州-4502,桂林-4503,梧州-4504,北海-4505,防城港-4506,钦州-4507,贵港-4508,玉林-4509,百色-4510,贺州-4511,河池-4512,来宾-4513,崇左-4514
海南:海口-4601,三亚-4602,三沙-4603,儋州-4604
重庆:重庆-50
四川:成都-5101,自贡-5103,攀枝花-5104,泸州-5105,德阳-5106,绵阳-5107,广元-5108,遂宁-5109,内江-5110,乐山-5111,南充-5113,眉山-5114,宜宾-5115,广安-5116,达州-5117,雅安-5118,巴中-5119,资阳-5120,阿坝-5132,甘孜-5133,凉山-5134
贵州:贵阳-5201,六盘水-5202,遵义-5203,安顺-5204,毕节-5205,铜仁-5206,黔西南-5223,黔东南-5226,黔南-5227
云南:昆明-5301,曲靖-5303,玉溪-5304,保山-5305,昭通-5306,丽江-5307,普洱-5308,临沧-5309,楚雄-5323,红河-5325,文山-5326,西双版纳-5328,大理-5329,德宏-5331,怒江-5333,迪庆-5334
西藏:拉萨-5401,日喀则-5402,昌都-5403,林芝-5404,山南-5405,那曲-5406,阿里-5425
陕西:西安-6101,铜川-6102,宝鸡-6103,咸阳-6104,渭南-6105,延安-6106,汉中-6107,榆林-6108,安康-6109,商洛-6110
甘肃:兰州-6201,嘉峪关-6202,金昌-6203,白银-6204,天水-6205,武威-6206,张掖-6207,平凉-6208,酒泉-6209,庆阳-6210,定西-6211,陇南-6212,临夏-6229,甘南-6230
青海:西宁-6301,海东-6302,海北-6322,黄南-6323,海南-6325,果洛-6326,玉树-6327,海西-6328
宁夏:银川-6401,石嘴山-6402,吴忠-6403,固原-6404,中卫-6405
新疆:乌鲁木齐-6501,克拉玛依-6502,吐鲁番-6504,哈密-6505,昌吉-6523,博尔塔拉-6527,巴音郭楞-6528,阿克苏-6529,克孜勒苏-6530,喀什-6531,和田-6532,伊犁-6540,塔城-6542,阿勒泰-6543`

type provinceEntry struct {
	Name   string
	Cities []cityEntry
}

type cityEntry struct {
	Name string
	Code int64
}

var provinceTable = parseProvinceTable(provinceCitiesText)

func parseProvinceTable(text string) []provinceEntry {
	var table []provinceEntry
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		name, citiesText, ok := strings.Cut(line, ":")
		if !ok {
			panic("省市代码数据格式错误: " + line)
		}
		p := provinceEntry{Name: name}
		for _, item := range strings.Split(citiesText, ",") {
			cityName, codeText, ok := strings.Cut(item, "-")
			if !ok {
				panic("城市代码数据格式错误: " + item)
			}
			code, err := strconv.ParseInt(codeText, 10, 64)
			if err != nil {
				panic("城市代码不是数字: " + item)
			}
			p.Cities = append(p.Cities, cityEntry{Name: cityName, Code: code})
		}
		table = append(table, p)
	}
	return table
}

// localEnvelope 与平台 API 响应格式保持一致：{"code":0,"message":"OK","data":...}
type localEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func writeLocalResult(opts *runner.Options, data any) error {
	b, err := json.Marshal(localEnvelope{Code: 0, Message: "OK", Data: data})
	if err != nil {
		return &runner.ExitError{Code: 2, Err: err}
	}
	if opts.Pretty {
		var buf bytes.Buffer
		if err := json.Indent(&buf, b, "", "  "); err == nil {
			b = buf.Bytes()
		}
	}
	fmt.Fprintln(opts.Stdout, string(b))
	return nil
}

type cityRow struct {
	Province string `json:"province"`
	City     string `json:"city"`
	Code     int64  `json:"code"`
}

// newProxyGroup 构建 proxy 模块命令组：API 命令 + 本地省市代码查询命令。
func newProxyGroup(opts *runner.Options) *cobra.Command {
	g := runner.NewGroup("proxy", "代理模块", proxySpecs, opts)
	g.AddCommand(newProxyProvincesCmd(opts), newProxyCitiesCmd(opts))
	return g
}

func newProxyProvincesCmd(opts *runner.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "provinces",
		Short: "获取平台代理可用的省份列表（本地数据）",
		Long: `获取平台代理可用的省份列表。
本命令使用内置的官方城市代码对照表，不调用 API，无需 --token 与 --app-id。
省名可用于 proxy cities --province 过滤；上海不在平台对照表内。`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			names := make([]string, 0, len(provinceTable))
			for _, p := range provinceTable {
				names = append(names, p.Name)
			}
			return writeLocalResult(opts, names)
		},
	}
}

func newProxyCitiesCmd(opts *runner.Options) *cobra.Command {
	c := &cobra.Command{
		Use:   "cities",
		Short: "获取城市代码列表（平台代理 proxyInfo 传城市代码）",
		Long: `获取城市代码列表；平台代理（proxyType=platform）的 proxyInfo 需传城市代码，login qr 同理。
本命令使用内置的官方城市代码对照表，不调用 API，无需 --token 与 --app-id。
--province/--city 需与对照表名称完全一致（如 广东、深圳，不带"省/市"后缀）；可组合过滤。
无匹配时退出码为 2；完整省份列表见 proxy provinces。`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			province, _ := cmd.Flags().GetString("province")
			city, _ := cmd.Flags().GetString("city")
			rows := make([]cityRow, 0)
			for _, p := range provinceTable {
				if province != "" && p.Name != province {
					continue
				}
				for _, c := range p.Cities {
					if city != "" && c.Name != city {
						continue
					}
					rows = append(rows, cityRow{Province: p.Name, City: c.Name, Code: c.Code})
				}
			}
			if len(rows) == 0 {
				return &runner.ExitError{Code: 2, Err: fmt.Errorf("未找到匹配的省市（province=%q city=%q）；可用省份见 proxy provinces", province, city)}
			}
			return writeLocalResult(opts, rows)
		},
	}
	c.Flags().String("province", "", "按省份过滤，如 广东")
	c.Flags().String("city", "", "按城市名过滤，如 深圳")
	return c
}
