-- /admin/organization-usage 组织用量报表日汇总。
-- 汇总严格镜像 usage_logs：按北京时间自然日 × 用户 × 分组 × 账号聚合，
-- 组织、部门、平台仍在查询时按 users/groups/accounts 当前值计算。
-- 迁移只创建结构与触发器，历史数据由后台作业逐日回填。

CREATE TABLE IF NOT EXISTS usage_user_daily_rollups (
    bucket_date DATE NOT NULL,
    user_id BIGINT NOT NULL,
    group_id BIGINT NOT NULL DEFAULT 0,
    account_id BIGINT NOT NULL,
    requests BIGINT NOT NULL,
    input_tokens BIGINT NOT NULL,
    output_tokens BIGINT NOT NULL,
    cache_creation_tokens BIGINT NOT NULL,
    cache_read_tokens BIGINT NOT NULL,
    actual_cost DECIMAL(24, 10) NOT NULL,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (bucket_date, user_id, group_id, account_id)
);

COMMENT ON TABLE usage_user_daily_rollups IS '组织用量报表：按北京时间自然日 × 用户 × 分组 × 账号聚合的 usage_logs。';
COMMENT ON COLUMN usage_user_daily_rollups.bucket_date IS 'Asia/Shanghai 自然日。';
COMMENT ON COLUMN usage_user_daily_rollups.group_id IS '0 表示 usage_logs.group_id 为 NULL。';

CREATE TABLE IF NOT EXISTS usage_user_rollup_state (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    closed_before DATE,
    reconciled_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE usage_user_rollup_state IS '组织用量日汇总的单行发布水位。';
COMMENT ON COLUMN usage_user_rollup_state.closed_before IS '早于该日期的日桶都已至少完整发布一次；NULL 表示未初始化。';
COMMENT ON COLUMN usage_user_rollup_state.reconciled_at IS '最近一次集群范围对账时间。';

INSERT INTO usage_user_rollup_state (id, closed_before)
VALUES (1, NULL)
ON CONFLICT (id) DO NOTHING;

-- 只追加：每个修改过去日期的事务为每天写一行自己的标记，日期不设唯一约束。
CREATE TABLE IF NOT EXISTS usage_user_rollup_dirty_days (
    id BIGSERIAL PRIMARY KEY,
    bucket_date DATE NOT NULL,
    marked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_usage_user_rollup_dirty_days_bucket_date
    ON usage_user_rollup_dirty_days (bucket_date);

COMMENT ON TABLE usage_user_rollup_dirty_days IS '原始用量在日桶发布后又发生变化、等待重建的北京时间日期；同一天可有多行。';

-- 登记某天为脏：只插入本事务自己的一行，不与其它标记冲突，因此不加锁、不等待、不会死锁。
-- 后台作业认领时只能删除已提交的标记；本事务未提交的标记不会被删，提交后留存，
-- 该天随后再次重建，所以作业不会用缺少本次改动的快照“永久”发布该天。
-- 同一事务内每天只登记一次（事务级设置随子事务回滚一起撤销），批量删除不会逐行写标记。
-- 只有写入过去日期的事务才会到达这里，当天的普通网关写入不写标记。
CREATE OR REPLACE FUNCTION mark_user_usage_rollup_dirty_day(p_day DATE)
RETURNS VOID
LANGUAGE plpgsql
AS $$
DECLARE
    marker TEXT := 'sub2api.user_rollup_dirty_' || to_char(p_day, 'YYYYMMDD');
BEGIN
    IF current_setting(marker, true) = 't' THEN
        RETURN;
    END IF;
    INSERT INTO usage_user_rollup_dirty_days (bucket_date) VALUES (p_day);
    PERFORM set_config(marker, 't', true);
END;
$$;

-- INSERT：语句级 + transition table，每条批量 INSERT 只执行一次。
-- 当天数据在 23:50 之前不登记；之后写入的当天数据提前登记，覆盖跨午夜提交的事务。
CREATE OR REPLACE FUNCTION mark_user_usage_rollup_dirty_after_insert()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    affected DATE;
BEGIN
    FOR affected IN
        SELECT DISTINCT (created_at AT TIME ZONE 'Asia/Shanghai')::date
        FROM inserted_usage_logs
        WHERE (created_at AT TIME ZONE 'Asia/Shanghai')::date
            < ((clock_timestamp() + INTERVAL '10 minutes') AT TIME ZONE 'Asia/Shanghai')::date
        ORDER BY 1
    LOOP
        PERFORM mark_user_usage_rollup_dirty_day(affected);
    END LOOP;
    RETURN NULL;
END;
$$;

-- UPDATE：语句级 + transition table。分区表上跨分区移动的 UPDATE 会被转换为
-- DELETE + INSERT，行级 AFTER UPDATE 与语句级 INSERT 触发器都不会触发，只有这里能看到新日期。
CREATE OR REPLACE FUNCTION mark_user_usage_rollup_dirty_after_update()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    affected DATE;
BEGIN
    FOR affected IN
        SELECT DISTINCT changed.bucket_date
        FROM old_usage_logs o
        JOIN new_usage_logs n ON n.id = o.id
        CROSS JOIN LATERAL (VALUES
            ((o.created_at AT TIME ZONE 'Asia/Shanghai')::date),
            ((n.created_at AT TIME ZONE 'Asia/Shanghai')::date)
        ) AS changed(bucket_date)
        WHERE (o.created_at, o.user_id, o.group_id, o.account_id, o.input_tokens, o.output_tokens,
               o.cache_creation_tokens, o.cache_read_tokens, o.actual_cost)
            IS DISTINCT FROM
              (n.created_at, n.user_id, n.group_id, n.account_id, n.input_tokens, n.output_tokens,
               n.cache_creation_tokens, n.cache_read_tokens, n.actual_cost)
        ORDER BY 1
    LOOP
        PERFORM mark_user_usage_rollup_dirty_day(affected);
    END LOOP;
    RETURN NULL;
END;
$$;

-- DELETE/UPDATE 行级触发器覆盖外键级联、分区表和直接分区写入。
CREATE OR REPLACE FUNCTION mark_user_usage_rollup_dirty_row()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    old_day DATE := (OLD.created_at AT TIME ZONE 'Asia/Shanghai')::date;
    new_day DATE;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        new_day := (NEW.created_at AT TIME ZONE 'Asia/Shanghai')::date;
        PERFORM mark_user_usage_rollup_dirty_day(LEAST(old_day, new_day));
        PERFORM mark_user_usage_rollup_dirty_day(GREATEST(old_day, new_day));
        RETURN NEW;
    END IF;
    PERFORM mark_user_usage_rollup_dirty_day(old_day);
    RETURN OLD;
END;
$$;

CREATE OR REPLACE FUNCTION reset_user_usage_rollups_after_truncate()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    DELETE FROM usage_user_daily_rollups;
    DELETE FROM usage_user_rollup_dirty_days;
    UPDATE usage_user_rollup_state
    SET closed_before = NULL,
        reconciled_at = NULL,
        updated_at = NOW()
    WHERE id = 1;
    RETURN NULL;
END;
$$;

DROP TRIGGER IF EXISTS usage_logs_user_rollup_dirty_insert ON usage_logs;
CREATE TRIGGER usage_logs_user_rollup_dirty_insert
AFTER INSERT ON usage_logs
REFERENCING NEW TABLE AS inserted_usage_logs
FOR EACH STATEMENT
EXECUTE FUNCTION mark_user_usage_rollup_dirty_after_insert();

DROP TRIGGER IF EXISTS usage_logs_user_rollup_dirty_update_statement ON usage_logs;
CREATE TRIGGER usage_logs_user_rollup_dirty_update_statement
AFTER UPDATE ON usage_logs
REFERENCING OLD TABLE AS old_usage_logs NEW TABLE AS new_usage_logs
FOR EACH STATEMENT
EXECUTE FUNCTION mark_user_usage_rollup_dirty_after_update();

DROP TRIGGER IF EXISTS usage_logs_user_rollup_dirty_delete ON usage_logs;
CREATE TRIGGER usage_logs_user_rollup_dirty_delete
AFTER DELETE ON usage_logs
FOR EACH ROW
EXECUTE FUNCTION mark_user_usage_rollup_dirty_row();

DROP TRIGGER IF EXISTS usage_logs_user_rollup_dirty_update ON usage_logs;
CREATE TRIGGER usage_logs_user_rollup_dirty_update
AFTER UPDATE OF created_at, user_id, group_id, account_id, input_tokens, output_tokens,
    cache_creation_tokens, cache_read_tokens, actual_cost ON usage_logs
FOR EACH ROW
WHEN (
    OLD.created_at IS DISTINCT FROM NEW.created_at
    OR OLD.user_id IS DISTINCT FROM NEW.user_id
    OR OLD.group_id IS DISTINCT FROM NEW.group_id
    OR OLD.account_id IS DISTINCT FROM NEW.account_id
    OR OLD.input_tokens IS DISTINCT FROM NEW.input_tokens
    OR OLD.output_tokens IS DISTINCT FROM NEW.output_tokens
    OR OLD.cache_creation_tokens IS DISTINCT FROM NEW.cache_creation_tokens
    OR OLD.cache_read_tokens IS DISTINCT FROM NEW.cache_read_tokens
    OR OLD.actual_cost IS DISTINCT FROM NEW.actual_cost
)
EXECUTE FUNCTION mark_user_usage_rollup_dirty_row();

DROP TRIGGER IF EXISTS usage_logs_user_rollup_reset_truncate ON usage_logs;
CREATE TRIGGER usage_logs_user_rollup_reset_truncate
AFTER TRUNCATE ON usage_logs
FOR EACH STATEMENT
EXECUTE FUNCTION reset_user_usage_rollups_after_truncate();
