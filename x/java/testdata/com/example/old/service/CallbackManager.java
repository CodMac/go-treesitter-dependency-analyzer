package com.example.service;

public class CallbackManager {
    public void register() {
        // 边界：方法体内部定义的局部类 (Local Class)
        class LocalValidator {
            public boolean isValid() {
                return true;
            }
        }

        // 边界：匿名内部类
        Runnable r = new Runnable() {
            @Override
            public void run() {
                System.out.println("Running");
            }
        };
    }
}