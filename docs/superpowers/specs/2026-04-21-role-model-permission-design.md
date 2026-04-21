# 角色权限设计：模型管理

**日期:** 2026-04-21  
**范围:** 超级管理员 vs 租户管理员对模型管理的权限边界

---

## 角色定义

系统仅有两种角色，以 `users.can_access_all_tenants` 字段区分：

| 角色 | `can_access_all_tenants` | 说明 |
|------|--------------------------|------|
| **超级管理员** (Super Admin) | `true` | 平台级管理员，可跨租户操作，拥有最高权限 |
| **租户管理员** (Tenant Admin) | `false` | 所属租户的管理员，默认租户即自己注册时创建的租户 |

> 每个用户注册时系统自动创建一个同名租户，用户即该租户的唯一管理员。

---

## 模型管理权限矩阵

| 操作 | 超级管理员 | 租户管理员 |
|------|-----------|-----------|
| 查看模型列表（本租户 + 内置） | ✅ | ✅ |
| 查看内置模型完整凭证（APIKey/BaseURL） | ✅ | ❌（字段置空） |
| 新建普通模型 | ✅ | ✅ |
| 新建时设置 `is_builtin = true` | ✅ | ❌（字段被忽略） |
| 编辑本租户普通模型 | ✅ | ✅ |
| 编辑内置模型（含去掉 builtin 标记） | ✅ | ❌ |
| 删除本租户普通模型 | ✅ | ✅ |
| 删除内置模型 | ✅ | ❌ |
| 切换 `is_builtin` 标记（设置/取消） | ✅ | ❌ |

---

## 内置模型（Builtin Model）语义

- `is_builtin = true` 的模型由超级管理员在平台层面创建，**所有租户均可见可用**。
- 内置模型的 `tenant_id` = 超级管理员所属租户 ID（并非全局 0，历史设计选择）。
- 租户管理员查询时 SQL 条件为 `tenant_id = ? OR is_builtin = true`，因此能看到但不拥有。
- 租户管理员**无法看到**内置模型的 APIKey/BaseURL（`hideSensitiveInfo` 对其置空）。
- 租户管理员**可以**在内置模型基础上自行添加同类型的自有模型（不影响内置模型）。

---

## 实现位置

### 后端

| 文件 | 逻辑 |
|------|------|
| `internal/types/user.go` | `User.CanAccessAllTenants bool` — 角色判断字段 |
| `internal/types/context_helpers.go` | `IsSuperAdmin(ctx)` / `UserFromContext(ctx)` — 上下文辅助函数 |
| `internal/middleware/auth.go` | JWT/API-Key 认证，将 `User` 注入 ctx |
| `internal/handler/model.go` | `hideSensitiveInfo(model, c)` — 超管可见完整凭证；`CreateModel`/`UpdateModel` 中 `is_builtin` 字段仅超管可写 |
| `internal/application/service/model.go` | `UpdateModel` / `DeleteModel` — 非超管操作内置模型返回错误 |

### 前端

| 文件 | 逻辑 |
|------|------|
| `frontend/src/stores/auth.ts` | `canAccessAllTenants` computed — 来自登录响应 |
| `frontend/src/views/settings/ModelSettings.vue` | `isSuperAdmin` 计算属性控制编辑/删除/菜单项可见性 |
| `frontend/src/components/ModelEditorDialog.vue` | `isBuiltin` 开关仅在 `canAccessAllTenants` 时显示 |

---

## 租户管理员的模型使用策略

1. **未自定义模型**：自动使用平台配置的内置模型（`is_builtin = true`）。
2. **自定义模型**：租户可新增本租户私有模型，在知识库/对话中选择使用。
3. 租户模型与内置模型**共存**，互不干扰。

---

## Bug 修复记录（2026-04-21）

**问题：** 超级管理员无法编辑内置模型，包括取消 `is_builtin` 标记。

**根因：**
- `service/model.go` `UpdateModel()` 和 `DeleteModel()` 无差别拦截所有内置模型操作，未区分角色。
- `handler/model.go` `hideSensitiveInfo()` 对所有用户隐藏内置模型凭证，导致超管编辑时 APIKey/BaseURL 被清空。
- `ModelSettings.vue` 前端 `editModel()` / `deleteModel()` / `getModelOptions()` 硬编码阻止内置模型操作，未判断是否超管。

**修复：**
- 后端服务层：`UpdateModel`/`DeleteModel` 增加 `types.IsSuperAdmin(ctx)` 检查，超管放行。
- 后端处理层：`hideSensitiveInfo` 增加 `gin.Context` 参数，超管跳过脱敏。
- 前端：`ModelSettings.vue` 引入 `useAuthStore`，三处操作改为 `model.isBuiltin && !isSuperAdmin.value` 条件。
