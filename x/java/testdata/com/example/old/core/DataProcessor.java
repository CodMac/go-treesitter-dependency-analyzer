package com.example.core;

import com.example.model.AbstractBaseEntity;

import java.util.List;

/**
 * 测试：Interface EXTEND Interfaces, Method Throws, Generic Parameters
 */
public interface DataProcessor<T extends AbstractBaseEntity<?>> extends Runnable, AutoCloseable {

    // 测试：返回类型引用与 Throws 引用
    List<T> processAll(String batchId) throws RuntimeException, Exception;

    // 测试：Java 8+ 默认方法
    default void stop() {
        System.out.println("Processor stopped.");
    }
}