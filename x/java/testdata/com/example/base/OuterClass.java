package com.example.base;

public class OuterClass {
    public static int count;

    // 静态初始化块
    static {
        count = 10;
        System.out.println("Static block initialized.");
    }

    // 实例初始化块
    {
        System.out.println("Instance block initialized.");
    }

    // 内部类 (Inner Class)
    public class InnerClass {
        public void display() {}
    }

    // 嵌套类 (Nested Static Class)
    public static class StaticNestedClass {
        public static void run() {}
    }

    public void scopeTest() {
        // 方法内部类 (Local Class)
        class LocalClass {
            void action() {}
        }
    }
}