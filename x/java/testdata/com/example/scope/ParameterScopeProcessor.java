package com.example.scope;

public class ParameterScopeTester {
    // 构造函数
    public ParameterScopeTester(String initialConfig) {
    }

    // 普通方法与变长参数
    public void execute(int times, String... labels) {
    }

    // 内部类中的方法参数
    class InnerWorker {
        void doWork(long duration) {
        }
    }
}