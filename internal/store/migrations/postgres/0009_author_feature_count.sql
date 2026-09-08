-- 0009: 功能数改为 AI 看代码判断（不再数审查/PR 数）。
-- review_author_reports 增加 feature_count：该作者在本次审查区间内被 AI 归属的
-- 功能/工作项数量（pi-agent 的 features 清单按 author 归集，未标注归 commit 作者）。
-- 0 = 历史数据，作者聚合统计时按「一条作者报告 = 1 个功能」回退。
ALTER TABLE review_author_reports ADD COLUMN IF NOT EXISTS feature_count INTEGER NOT NULL DEFAULT 0;
