-- 000009_learner_permissions.down.sql
--
-- 回滚 000009：删 4 条 learner 写权限的 brand_role_permissions / role_template_permissions
-- 映射，再删 permission 行本身。learner.view（000003）不动。表结构本批未动，无需回滚。

DELETE FROM brand_role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE code IN (
        'learner.create', 'learner.edit', 'learner.delete', 'learner.freeze'
    )
);

DELETE FROM role_template_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE code IN (
        'learner.create', 'learner.edit', 'learner.delete', 'learner.freeze'
    )
);

DELETE FROM permissions WHERE code IN (
    'learner.create', 'learner.edit', 'learner.delete', 'learner.freeze'
);
