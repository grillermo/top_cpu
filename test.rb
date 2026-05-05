Process.setproctitle('[stress test]')
def stress_the_gc
  loop do
    # Creating a large array of strings and immediately discarding it
    100_000.times { "cpu_killer_#{rand(1000)}".to_sym }
  end
end

stress_the_gc
