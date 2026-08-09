# @yueli/identifier

站群 JavaScript/TypeScript 的普通 Identifier 入口。它与 Go `identifier` 共享
`contracts/identifier/profiles.v1.json`：

- `newUUID` / `parseUUID`：RFC UUIDv7 writer 与严格 canonical parser；
- `deriveUUID`：UUIDv5 确定性派生；
- `newKey` / `parseKey`：版本化 Public Key Profile；
- `allocateKey`：最多 8 次的原子唯一占用循环。

```ts
import {
  Claimed,
  Collision,
  CompactURLV1,
  allocateKey,
  newUUID,
} from "@yueli/identifier";

const entityID = newUUID();
const publicKey = await allocateKey(CompactURLV1, async (candidate) => {
  const inserted = await insertWithUniqueConstraint(candidate);
  return inserted ? Claimed : Collision;
});
```

Secret、Trace、幂等语义、业务编号、handle、slug 和临时 DOM key 不属于本包。

发布包只导出编译后的 ESM JavaScript 与声明文件；`src/*.ts` 不是消费者运行时入口。
