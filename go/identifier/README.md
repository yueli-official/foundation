# Identifier

`identifier` 是站群统一标识 Module。它把 UUID 实现、公开 Key 的随机编码、严格解析和碰撞分配循环隐藏在一个
稳定 Interface 后，产品不再直接选择算法或字符集。

## Interface

- `New()` / `MustNew()`：创建 RFC 9562 UUIDv7；适用于实体、事件、任务和投递记录。
- `Parse()`：只接受规范小写、连字符分隔的 UUID 文本。
- `Derive()`：以稳定 namespace 和 canonical bytes 创建 UUIDv5。
- `CompactURLV1`：8 位 Base58 公开 URL 定位符，约 47 bit 空间。
- `HumanCodeV1`：10 位无歧义 Base32 人工码，50 bit 空间。
- `OpaquePublicV1`：16 位无歧义 Base32 公开引用，80 bit 空间。
- `Allocate()`：通过产品拥有的原子 claim Adapter 完成最多八次碰撞重试。

```go
entityID, err := identifier.New()
if err != nil {
    return err
}

publicKey, err := identifier.Allocate(ctx, identifier.CompactURLV1,
    func(ctx context.Context, candidate identifier.Key) (identifier.ClaimResult, error) {
        // INSERT the owning row under its named UNIQUE constraint.
        return identifier.Claimed, nil
    })
```

`KeyProfile.New()` 只产生候选值；需要唯一性的产品必须通过 `Allocate()` 或等价的原子唯一约束闭环完成分配。

## Non-responsibilities

- bearer capability、邀请凭证、重置链接和登录 token；
- W3C trace/span ID；
- 客户端幂等合同与响应重放；
- 订单号、发票号、审计序列等业务编号；
- handle、slug、分页 cursor 和 Sqids 整数编码。

这些值可能也是字符串，但拥有不同的保密、生命周期、存储和错误语义，不能复用公开 Identifier Interface。

