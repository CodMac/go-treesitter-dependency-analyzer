package com.example.member;

// 目标：验证 QN 的唯一性（通过参数类型区分）、构造函数识别、返回值提取。
public class MethodTest {
    private String name;

    public MethodTest() {} // Constructor

    public void exec(int i) {}                // QN 应含 (int)
    public String exec(String s) { return s; } // QN 应含 (String)
    public void exec(int i, String s) {}       // QN 应含 (int,String)
}