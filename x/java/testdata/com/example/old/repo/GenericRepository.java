package com.example.repo;

import java.io.Serializable;
import java.util.List;

/**
 * 边界：泛型 T 必须同时实现 Serializable 和 Cloneable
 */
public interface GenericRepository<T extends Serializable & Cloneable> {

    // 测试：复杂的泛型返回类型和通配符
    List<? extends T> findAllByCriteria(List<? super T> criteria);

    // 测试：方法级别的泛型定义
    <E extends Exception> void executeOrThrow(E exception) throws E;
}