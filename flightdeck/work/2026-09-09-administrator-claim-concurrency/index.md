# 首位管理员并发认领稳定性

## Status

Finished

## Goal

多个进程竞争认领时仅产生一位管理员，其余请求稳定返回冲突；重启保留授权。

## Current

Paste 接入发现投影重建的 Serializable 事务在竞争时可能返回 40001。对序列化失败与死锁进行有限重试，保留其他错误和上下文取消。共享数据库测试增加到两个 Adapter、20 个竞争者。

## Verification

Paste 的独立 schema 数据库集成测试已通过，覆盖匿名拒绝、单一赢家、重启与已有管理员。Foundation 直接数据库回归也已通过（两个 Adapter、20 位竞争者、重启及管理员丢失不得重新认领）。

## Boundary

本地源码修改；未发布 Foundation 新版本。

## Next

随下一次获授权的 Foundation Go 制品发布交付；本次不发布。
