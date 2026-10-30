# china-admin-divisions

行政区划查询、可重现二进制数据生成、Go embed 和浏览器单请求流式加载。

维护源为 `data.json`，提交的 `data.bin.gz` 由生成器产生。运行时不解析 JSON，首次查询解压并建立搜索索引；HTTP 快照只驻留压缩后的传输数据，不保留未压缩二进制或 JSON。内嵌数据损坏直接 panic。更新外部 JSON 失败返回错误，保留旧快照。

## 生成与验证

生成器和 CI 固定 Go **1.27.2**；更换 Go/压缩库版本可能改变 gzip 字节，升级时应统一重新生成。核心查询、编码和 net/http 接口仅使用标准库，`ginhandler` 子包依赖 Gin。

```sh
GOTOOLCHAIN=go1.27.2 go generate ./...
go test ./...
go test -race ./...
go vet ./...
npm ci
npm run typecheck
npm run generate:browser
npm test
```

Node 测试使用 24。生成器不写时间戳、源路径或机器信息；gzip 使用标准库 BestCompression，MTIME=0、OS=255、空文件名/注释。父节点先于子节点，采用深度优先遍历，同级按代码数值升序；数值相同但位数不同则按原代码字典序打破平局。代码位数单独保留，因此前导零不会丢失。不依赖 JSON 数组、字段或 Go map 顺序。

生成器校验重复代码、字段范围、UTF-8、父节点存在及父子层级关系；父子层级严格递增，因此拒绝环。允许跳级，不按代码前缀猜测关系。

## Go

```go
import (
    "net/http"
    division "github.com/iseki0/china-admin-divisions"
)

index := division.Default() // 内嵌数据加载失败 panic，无需处理 error
record, found := index.Lookup("110000")
hits, err := index.Search("bei jing", division.Options{Limit: 20})

http.Handle("/admin-divisions.bin", division.Handler(nil, division.HTTPOptions{}))
```

`Loader` 零值使用 embed；`NewLoader(jsonBytes)` 创建独立快照；`loader.LoadFile(path)` 显式读取 JSON、校验、排序、编码、压缩后原子替换。每次操作使用 `loader.Current()` 获取一致快照。不会自动监听文件，不在 HTTP 请求中读取磁盘。首次成功的显式加载可直接初始化 Loader。

HTTP GET 一次返回全量。默认**无条件强制 gzip**，包括客户端未声明或拒绝 gzip 的情况。设置 `HTTPOptions{DisableForceGzip: true}` 后按质量值协商 gzip/identity；均不可接受时返回 406，identity 仅在该请求期间解压，不驻留内存。默认无条件强制的行为不遵循客户端拒绝 gzip 的声明，是有意提供的策略开关。

响应使用 `application/octet-stream`、`Content-Encoding: gzip`、压缩表示的 SHA-256 强 ETag、`Cache-Control: public, no-cache`。支持 HEAD、If-None-Match 列表/弱标签/`*` 和 304，其他方法 405；协商模式设置 `Vary: Accept-Encoding`，identity 使用自己的 ETag。压缩中间件应跳过已编码响应；宿主负责路由、CORS 和鉴权。

Gin 可使用 `ginhandler.New(loader)`，或 `ginhandler.New(loader, options)`，注册 GET/HEAD。

搜索支持中文、代码、拼音和首字母；排序依次为精确/前缀/连续包含、弃用状态、代码字典序。忽略大小写、空白和声调，ü/u:/v 等价。Limit=0 默认20，正数无固定上限，负数报错；CodePrefix 仅允许 ASCII 数字。索引并发只读，返回记录不影响索引。弃用状态保留 false/"suspected"/true 三态。

## 浏览器

浏览器入口无运行时依赖，通过 `@iseki0/china-admin-divisions` 导入。仓库提交 `dist/decoder.js` 和类型声明，Git 依赖安装不需要执行构建脚本。维护源为 `frontend/decoder.ts`，修改后执行 `npm run generate:browser`。一次 fetch，浏览器自动根据 HTTP Content-Encoding 解压，解析器通过 ReadableStream 分批解析，不能再手动解压。

```ts
import { loadDivisions, decodeDivisions } from '@iseki0/china-admin-divisions';

const nodes = await loadDivisions('/admin-divisions.bin', { signal });
// 返回数组中的 parentIndex 可直接访问父节点；0xFFFFFFFF 表示根。
```

需要边收边填时使用异步迭代器：

```ts
const response = await fetch('/admin-divisions.bin', { signal, cache: 'no-cache' });
if (!response.ok || !response.body) throw new Error('行政区划数据加载失败');
const nodes = [];
for await (const batch of decodeDivisions(response.body, { signal, batchSize: 512 })) {
  for (const node of batch) {
    nodes.push(node);
    // parentIndex 已可用，可在这里填充父子索引。
  }
}
// 迭代正常结束后才将这份数据标记为完整。
```

迭代器的批次在 EOF 校验通过前均是临时数据；中途网络失败、截断或校验失败时，应丢弃本次临时索引。`loadDivisions` 只在全量成功后返回。支持取消请求与消费；提前退出迭代时取消上游。每批让出事件循环，不逐条触发 UI 渲染。可在 Worker 中复用解析器；第一版没有 UI 框架或持久化缓存依赖。

## 二进制 v1

全部整数为小端序。无额外对齐填充，无字典、对象图、JSON 或运行时 schema。

文件头（10 字节）：

| 字段 | 类型 |
| --- | --- |
| magic | ASCII `CADB`，4字节 |
| 格式版本 | uint16，1 |
| 记录总数 | uint32 |

每条记录的固定头为20字节，随后紧接名称和拼音字节：

| 偏移 | 字段 | 类型 |
| --- | --- | --- |
| 0 | code | uint64，范围 0..999999999999 |
| 8 | parentIndex | uint32，根为 0xFFFFFFFF |
| 12 | level | uint8，1..4，保留跳级信息 |
| 13 | deprecated | uint8，0未标记、1疑似、2弃用 |
| 14 | codeWidth | uint8，1..12，保留前导零 |
| 15 | reserved | uint8，必须为0 |
| 16 | nameLength | uint16，UTF-8 字节数 |
| 18 | pinyinLength | uint16，UTF-8 字节数 |
| 20 | name | nameLength 字节 |
| 随后 | pinyin | pinyinLength 字节 |

前端使用两个 uint32 计算代码 `high * 2**32 + low`，所有合法代码都在 Number 精确范围内；按 codeWidth 补零后以字符串提供给业务。网络 chunk 可在任意字段或 UTF-8 字符中间截断，解析器保留未完成记录的尾部。

二进制版本与数据版本分开：格式版本在文件头，数据内容由 ETag 标识，数据来源版本见 [DATA.md](DATA.md)。不在格式头嵌入构建时间。
