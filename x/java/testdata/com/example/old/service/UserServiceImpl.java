package com.example.service;

import com.example.model.AbstractBaseEntity;
import com.example.core.DataProcessor;
import com.example.annotation.Loggable;

import java.util.ArrayList;
import java.util.List;
import java.util.UUID;

/**
 * 测试：Class EXTEND Class, IMPLEMENT Interface, Call, Create, Cast, Field Access
 */
@Loggable
public class UserServiceImpl extends AbstractBaseEntity<String> implements DataProcessor<AbstractBaseEntity<String>> {

    private final List<String> cache = new ArrayList<>();

    public UserServiceImpl() {
        // 测试：Field Access (static)
        this.id = UUID.randomUUID().toString();
    }

    @Override
    public List<AbstractBaseEntity<String>> processAll(String batchId) throws RuntimeException {
        // 测试：Object Creation (Create)
        List<AbstractBaseEntity<String>> results = new ArrayList<>();

        // 测试：Method Call (Call)
        String upperBatch = batchId.toUpperCase();

        // 测试：Cast Expression (Cast)
        Object rawData = "some data";
        String converted = (String) rawData;

        return results;
    }

    @Override
    public void run() {
        // 实现 Runnable
    }

    @Override
    public void close() throws Exception {
        // 实现 AutoCloseable
    }
}