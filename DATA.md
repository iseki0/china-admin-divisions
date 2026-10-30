# 数据来源与维护边界

本仓库初始数据及查询实现从 `iseki0/industry` 的 `ba7eb949` 迁移；原仓库未作修改。保留 `data.json` 作为后续迭代维护源，生成的 gzip 二进制与 JSON 一起提交，消费者无需先运行生成器。

数据来自原仓库 `packages/admin-division-react/data.json`，为平铺的父子记录，并非嵌套 JSON。初始版本为 Xzqh20251231 全国年度快照，加原仓库截至2026-10的部分审计和补充；不是全国2026年现行名录。保留原名称、拼音、弃用状态、显式层级和父级关系，不通过代码前缀重建层级。

官方入口：https://dmfw.mca.gov.cn/XzqhVersionPublish.html

初始上游 CSV：https://github.com/wujingke/division_codes/blob/main/output/csv/division_codes.csv

完整来源、审计范围及拼音覆盖依据仍见原仓库：
https://github.com/iseki0/industry/tree/ba7eb949/references/data/administrative-divisions

上游 CSV 未发现许可证；本次迁移不声明新的数据授权，也不改变既有再分发授权边界。拼音为机器派生值并包含人工覆盖，尚未逐条审校；false 仅表示未标记弃用，不代表已核验现行性。雄安新区133100的level=2用于数据组织，不认定其法定行政级别。无有效代码的占位记录不编造代码。

更新数据时编辑或替换 data.json，保留来源依据，然后运行固定工具链生成器并检查测试。JSON 行排列不影响生成产物；建议维护时也保持相同的父节点在前顺序，方便审查。
