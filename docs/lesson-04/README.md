# 第 4 课验收记录：可追溯知识库检索

## 交付物

- 三种切分：`auto`（默认 800 字、重叠 80）、`custom`（100–2000 字、重叠 0–50%，可移除 URL/邮箱、折叠空白）、`hierarchy`（按 `#/##/###` 分章，超长再窗口切分）。
- MySQL `knowledge_chunks` 保存正文、序号、字符偏移、索引状态与策略；`FULLTEXT ... WITH PARSER ngram` 支撑中文关键字检索。
- Qdrant collection `campusclaw_chunks`，余弦距离；point id 等于 `knowledge_chunks.id`，payload 只含 `class_id/material_id/knowledge_entry_id/chunk_id/chunk_index`。
- `POST /api/search`：`keyword`、`vector`、`hybrid`；混合模式按名次 RRF（k=60）。
- `POST /api/ask`：先混合检索取前 4 条；无命中直接返回固定文案，**不调用模型**。
- `POST /api/materials/{id}/reindex`：按本次策略重建；`GET /api/materials/{id}/vectors` 只展示切片元数据，不暴露向量分量。

## 关键数据形态

```sql
-- 正文在 MySQL
SELECT id,material_id,chunk_index,CHAR_LENGTH(chunk_text),index_status,chunk_strategy
FROM knowledge_chunks ORDER BY id;

-- 向量库只存向量与标识
-- point id = chunk_id，payload:
-- {"class_id":1,"material_id":1,"knowledge_entry_id":1,"chunk_id":4,"chunk_index":0}
```

## 本地验收命令与结果

```bash
# 数学材料中确有“去括号”依据
POST /api/search {"q":"去括号","mode":"keyword"}
# -> 200 ok，命中数学自学笔记切片

POST /api/search {"q":"去括号","mode":"vector"}
# -> 200 ok，命中同一切片，score 约 0.37

POST /api/search {"q":"去括号","mode":"hybrid"}
# -> 200 ok，该切片同时具有 keyword_rank 与 vector_rank，融合分 1/(60+1)+1/(60+1)

# 无关问题
POST /api/search {"q":"请讲一个和课程无关的笑话","mode":"hybrid"}
# -> 200 no_evidence，hits []

POST /api/ask {"q":"今天天气怎么样"}
# -> 200 {"status":"no_evidence","answer":"资料中未找到相关内容","citations":[]}
#    模型调用次数 0（代码路径在检索后直接返回）

# 仅存在于 B 班的词
A 班 POST /api/search {"q":"量子纠缠草莓","mode":"hybrid"}
# -> 200 no_evidence，hits []
B 班 POST /api/search {"q":"量子纠缠草莓","mode":"hybrid"}
# -> 200 ok，命中《B班物理笔记》

# 跨班详情
GET /api/materials/3（B 班材料，用 A 班令牌）
# -> 404
```

## 失败路径与边界

- 空查询：400。
- `limit`/`offset` 为负：400；`offset` 超出结果集：200 `status=ok`、空数组（不是 `no_evidence`）。
- Qdrant 不可用：`keyword` 仍可返回正文；`vector`/`hybrid` 返回 503，不编造相似度。
- 嵌入失败：材料与 `knowledge_entries` 保留，切片标 `failed`，不写入不完整向量；重试可重建。
- 跨班：检索 200 空结果，不返回 403，避免泄露材料存在性。
- 引用编号先剔除空正文再连续编号，回答中的 `[1]` 与 citations 顺序一致。

## 离线模式说明

未配置 `EMBEDDING_API_KEY` 时，使用字符二元组哈希向量作为演示回退，并额外要求查询与切片存在非停用词二元组重合，避免把“今天天气”这类无关问题误判为有依据。配置课程网关后自动使用 `/v1/embeddings` 与 `/v1/chat/completions`，阈值恢复为配置的 0.35。

## 自动化验证

`apitest/run_all.py` 在 2026-10-27 全量运行：35 个用例通过，0 失败；报告写入 `apitest/report.html`（不入库）。
