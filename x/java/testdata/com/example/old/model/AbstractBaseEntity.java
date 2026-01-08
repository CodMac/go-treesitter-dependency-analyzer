package com.example.model;

import java.io.Serializable;
import java.util.Date;

/**
 * 基础实体类：测试泛型、继承、字段、方法及内部类
 */
public abstract class AbstractBaseEntity<ID> implements Serializable {

    protected ID id;

    private Date createdAt;

    public ID getId() {
        return id;
    }

    public void setId(ID id) {
        this.id = id;
    }

    /**
     * 内部类：验证 QN 递归拼接
     */
    public static class EntityMeta {
        private String tableName;

        public String getTableName() {
            return tableName;
        }
    }
}